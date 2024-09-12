package usercontroller

import (
	"fmt"
	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/handle"
	"observeddb-go-api/internal/model"
	"observeddb-go-api/internal/responder"
	"observeddb-go-api/internal/utils/hash"
	"observeddb-go-api/internal/utils/helper"
	"observeddb-go-api/internal/utils/tokens"
	"observeddb-go-api/internal/utils/validate"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UserController struct {
	DB       *gorm.DB
	AuthCfg  cfg.AuthTokenConfig
	ResetCfg cfg.ResetTokenConfig
	Mailer   *responder.Mailer
}

func NewUserController(resourceHandle *handle.ResourceHandle) *UserController {
	return &UserController{
		DB:       resourceHandle.Gorm,
		AuthCfg:  resourceHandle.AuthCfg,
		ResetCfg: resourceHandle.ResetCfg,
		Mailer:   resourceHandle.Mailer,
	}
}

type loginResponse struct {
	tokens.AuthTokens
	Role      string     `json:"role" example:"user"`                       // User role
	LastLogin *time.Time `json:"last_login" example:"2021-07-01T12:00:00Z"` // Last login time
} //	@name	LoginResponse

func newLoginResponse(tokens *tokens.AuthTokens, role string, lastLogin *time.Time) *loginResponse {
	result := &loginResponse{
		AuthTokens: *tokens,
		Role:       role,
		LastLogin:  lastLogin,
	}

	return result
}

func switchRole(dbRole string, requestedRole *string) (string, error) {
	if requestedRole == nil || *requestedRole == dbRole {
		return dbRole, nil
	}

	err := validate.CanSwitchToRole(*requestedRole, dbRole)
	return *requestedRole, fmt.Errorf("cannot switch to role: %w", err)
}

// @Summary		Login for the API to get JWT token
// @Description	Acciqures a JWT token for the user to access the API
// @Description	Only active users can login
// @Description	Users can downgrade their role by providing the role in the request (optional)
// @Tags			Login
// @Produce		json
// @Param			request	body		usercontroller.LoginQuery						true	"Request body"
// @Success		200		{object}	handle.jsendSuccess[loginResponse]				"JWT token"
// @Failure		401		{object}	handle.jsendFailure[handle.errorResponse]		"Unauthorized"
// @Failure		422		{object}	handle.jsendFailure[handle.validationResponse]	"Bad query format"
// @Failure		403		{object}	handle.jsendFailure[handle.errorResponse]		"User is not active"
// @Failure		500		{object}	handle.jSendError								"Internal server error"
//
// @Router			/user/login [post]
func (uc *UserController) Login(c *gin.Context) {
	type Query struct {
		Login    string  `json:"login" binding:"required" example:"joe@me.com"`
		Password string  `json:"password" binding:"required" example:"password"`
		Role     *string `json:"role" binding:"omitempty,oneof=admin user approver" example:"user"`
	} //	@name	LoginQuery

	var query Query
	if !handle.JSONBind(c, &query) {
		return
	}

	user, err := model.GetUserByEmail(uc.DB, query.Login)
	if err != nil {
		handle.UnauthorizedError(c, "Invalid ceredentials")
		return
	}

	if user.PwdHash == nil {
		handle.UnauthorizedError(c, "Invalid ceredentials")
		return
	}

	if validPwd, _ := hash.Check(*user.PwdHash, query.Password); !validPwd {
		handle.UnauthorizedError(c, "Invalid credentials")
		return
	}

	if user.Status != "active" {
		handle.ForbiddenError(c, "User account is not active")
		return
	}

	newRole, err := switchRole(user.Role, query.Role)
	if err != nil {
		handle.ForbiddenError(c, "Unauthorized role access")
		return
	}

	claims := tokens.CustomClaims{
		ID:    user.ID,
		Email: user.Email,
		Role:  newRole,
	}
	token, err := tokens.CreateAuthTokens(&claims, &uc.AuthCfg)
	if err != nil {
		handle.ServerError(c, err)
		return
	}

	res := newLoginResponse(token, newRole, user.LastLogin)
	_ = user.UpdateLastLogin(uc.DB)

	handle.Success(c, res)
}

// @Summary		Refresh JWT token
// @Description	Refreshes the JWT token for the user to access the API
// @Tags			Login
// @Produce		json
// @Param			request	body		usercontroller.RefreshQuery						true	"Request body"
// @Success		200		{object}	handle.jsendSuccess[loginResponse]				"JWT token"
// @Failure		401		{object}	handle.jsendFailure[handle.errorResponse]		"Unauthorized"
// @Failure		422		{object}	handle.jsendFailure[handle.validationResponse]	"Bad query format"
// @Failure		403		{object}	handle.jsendFailure[handle.errorResponse]		"Not active/role invalid/user deleted"
// @Failure		500		{object}	handle.jSendError								"Internal server error"
//
// @Router			/user/refresh-token [post]
func (uc *UserController) RefreshToken(c *gin.Context) {
	type Query struct {
		Token string `json:"refresh_token" binding:"required" example:"my_refresh_token"`
	} //	@name	RefreshQuery

	var query Query
	if !handle.JSONBind(c, &query) {
		return
	}

	claims, err := tokens.CheckRefreshToken(query.Token, &uc.AuthCfg)
	if err != nil {
		handle.UnauthorizedError(c, "Invalid refresh token")
		return
	}

	user, err := model.GetUserByID(uc.DB, claims.ID)
	if err != nil {
		handle.ForbiddenError(c, "User not found")
		return
	}

	if user.Status != "active" {
		handle.ForbiddenError(c, "User account is not active")
		return
	}

	if err = validate.CanSwitchToRole(claims.Role, user.Role); err != nil {
		handle.ForbiddenError(c, "Unauthorized role access")
		return
	}

	updatedClaims := tokens.CustomClaims{
		ID:    user.ID,
		Email: user.Email,
		Role:  claims.Role,
	}
	newToken, err := tokens.CreateAuthTokens(&updatedClaims, &uc.AuthCfg)
	if err != nil {
		handle.ServerError(c, err)
		return
	}

	res := newLoginResponse(newToken, user.Role, user.LastLogin)
	_ = user.UpdateLastLogin(uc.DB)
	handle.Success(c, res)
}

// @Summary		Change password for the user
// @Description	Changes the password for the user. The old password must be provided.
// @Description	The new password will be active on the next login.
// @Tags			User
// @Produce		json
// @Param			request	body		usercontroller.ChangePwdQuery					true	"Request body"
// @Success		200		{object}	handle.jsendSuccess[map[string]string]			"Password changed"
// @Failure		401		{object}	handle.jsendFailure[handle.errorResponse]		"Unauthorized"
// @Failure		400		{object}	handle.jsendFailure[handle.errorResponse]		"Wrong old password/invalid new password"
// @Failure		422		{object}	handle.jsendFailure[handle.validationResponse]	"Bad query format"
// @Failure		500		{object}	handle.jSendError								"Internal server error"
//
// @Security		Bearer
//
// @Router			/user/password [patch]
func (uc *UserController) ChangePwd(c *gin.Context) {
	type Query struct {
		OldPassword string `json:"old_password" binding:"required" example:"old_password"` // Old password
		NewPassword string `json:"new_password" binding:"required" example:"new_password"` // New password
	} //	@name	ChangePwdQuery

	var query Query
	if !handle.JSONBind(c, &query) {
		return
	}

	id := c.GetUint("user_id")
	user, err := model.GetUserByID(uc.DB, id)
	if err != nil {
		handle.ServerError(c, err)
		return
	}

	if validPwd, _ := hash.Check(*user.PwdHash, query.OldPassword); !validPwd {
		handle.BadRequestError(c, "Invalid old password")
		return
	}

	if err = validate.Password(query.NewPassword); err != nil {
		handle.BadRequestError(c, "Invalid new password")
		return
	}

	newPwdHash, err := hash.Create(query.NewPassword)
	if err != nil {
		handle.ServerError(c, err)
		return
	}

	user.PwdHash = &newPwdHash
	if err = user.Save(uc.DB); err != nil {
		handle.ServerError(c, err)
		return
	}

	handle.Success(c, gin.H{"message": "Password changed"})
}

// @Summary		Request password reset
// @Description	Requests a password reset for the user. A password reset token will be sent to the user's email.
// @Description	Password reset tokens are valid for a limited time.
// @Description	The API will always return the same message (200) to prevent email enumeration.
// @Tags			Login
// @Produce		json
// @Param			request	body		usercontroller.ResetPwdQuery					true	"Request body"
// @Success		200		{object}	handle.jsendSuccess[map[string]string]			"Password reset token sent"
// @Failure		422		{object}	handle.jsendFailure[handle.validationResponse]	"Bad query format"
// @Failure		500		{object}	handle.jSendError								"Internal server error"
//
// @Router			/user/password/reset [post]
func (uc *UserController) ResetPwd(c *gin.Context) {
	type Query struct {
		Email string `json:"email" binding:"required,email" example:"joe@me.com"` // Email address
	} //	@name	ResetPwdQuery

	var query Query
	if !handle.JSONBind(c, &query) {
		return
	}

	// We don't want to leak information about registered emails so we always return the same message
	const defaultMsg = "If your email is registered, you will receive a password reset token."
	user, err := model.GetUserByEmail(uc.DB, query.Email, "PwdReset")
	if err != nil {
		handle.Success(c, gin.H{"message": defaultMsg})
		return
	}

	if user.Status != "active" {
		handle.Success(c, gin.H{"message": defaultMsg})
		return
	}

	if user.PwdReset != nil {
		if err = validate.QueryRetry(user.PwdReset.UpdatedAt, uc.ResetCfg.RetryInterval); err != nil {
			handle.Success(c, gin.H{"message": defaultMsg})
			return
		}
	} else {
		user.PwdReset = &model.UserPwdReset{UserID: user.ID}
	}

	resetTokens, err := tokens.CreateResetTokens()
	if err != nil {
		handle.ServerError(c, err)
		return
	}
	user.PwdReset.ResetTokenHash = resetTokens.TokenHash
	user.PwdReset.TokenExpiry = time.Now().Add(uc.ResetCfg.ExpirationTime)

	if err = uc.DB.Transaction(func(tx *gorm.DB) error {
		if createErr := tx.Save(user.PwdReset).Error; createErr != nil {
			handle.ServerError(c, createErr)
			return gorm.ErrInvalidTransaction
		}

		mailerErr := uc.Mailer.SendPasswordResetEmail(
			"User",
			query.Email,
			resetTokens.Token,
			user.PwdReset.TokenExpiry,
		)
		if mailerErr != nil {
			handle.ServerError(c, mailerErr)
			return gorm.ErrInvalidTransaction
		}

		return nil
	}); err != nil {
		return
	}

	handle.Success(c, gin.H{"message": defaultMsg})
}

// @Summary		Confirm password reset or first password set
// @Description	Confirms a password reset or first password set for the user.
// @Description	The API will always return the same message (400) on auth errors to prevent email enumeration.
// @Tags			Login
// @Produce		json
// @Param			request	body		usercontroller.ResetConfirmPwdQuery				true	"Request body"
// @Success		200		{object}	handle.jsendSuccess[map[string]string]			"Password reset"
// @Failure		400		{object}	handle.jsendFailure[handle.errorResponse]		"Bad request/invalid token"
// @Failure		422		{object}	handle.jsendFailure[handle.validationResponse]	"Bad query format"
// @Failure		500		{object}	handle.jSendError								"Internal server error"
// @Failure		403		{object}	handle.jsendFailure[handle.errorResponse]		"Token expired"
//
// @Router			/user/password/reset/confirm [post]
// @Router			/user/password/reset/init [post]
func (uc *UserController) ResetPwdConfirm(c *gin.Context) {
	type Query struct {
		Token    string `json:"token" binding:"required" example:"my_reset_token"`   // Reset token
		Email    string `json:"email" binding:"required,email" example:"joe@me.com"` // Email address
		Password string `json:"password" binding:"required" example:"my_new_pwd"`    // New password
	} //	@name	ResetConfirmPwdQuery

	var query Query
	if !handle.JSONBind(c, &query) {
		return
	}

	// We don't want to leak information about registered emails so we always return the same message
	const defaultMsg = "Invalid reset token"
	user, err := model.GetUserByEmail(uc.DB, query.Email, "PwdReset")
	if err != nil {
		handle.BadRequestError(c, defaultMsg)
		return
	}

	if user.PwdReset == nil {
		handle.BadRequestError(c, defaultMsg)
		return
	}

	if validToken, _ := hash.Check(user.PwdReset.ResetTokenHash, query.Token); !validToken {
		handle.BadRequestError(c, defaultMsg)
		return
	}

	if err = validate.TokenExpiry(user.PwdReset.TokenExpiry); err != nil {
		handle.ForbiddenError(c, "Token expired")
		return
	}

	if err = validate.Password(query.Password); err != nil {
		handle.BadRequestError(c, fmt.Sprintf("Invalid password: %s", err))
		return
	}

	newPwdHash, err := hash.Create(query.Password)
	if err != nil {
		handle.ServerError(c, err)
		return
	}

	user.PwdHash = &newPwdHash
	err = uc.DB.Transaction(func(tx *gorm.DB) error {
		if err = user.Save(tx); err != nil {
			handle.ServerError(c, err)
			return gorm.ErrInvalidTransaction
		}

		if err = tx.Unscoped().Delete(user.PwdReset).Error; err != nil {
			handle.ServerError(c, err)
			return gorm.ErrInvalidTransaction
		}

		return nil
	})

	if err != nil {
		return
	}

	handle.Success(c, gin.H{"message": "Password reset"})
}

// @Summary		Request email change for the user
// @Description	Requests an email change for the user. An email change token will be sent to the new email address.
// @Tags			User
// @Produce		json
// @Param			request	body		usercontroller.ChangeEmailQuery					true	"Request body"
// @Success		200		{object}	handle.jsendSuccess[map[string]string]			"Email change request token sent"
// @Failure		401		{object}	handle.jsendFailure[handle.errorResponse]		"Unauthorized"
// @Failure		400		{object}	handle.jsendFailure[handle.errorResponse]		"Bad request/invalid email/already in use"
// @Failure		422		{object}	handle.jsendFailure[handle.validationResponse]	"Bad query format"
// @Failure		500		{object}	handle.jSendError								"Internal server error"
//
// @Security		Bearer
//
// @Router			/user/email [patch]
func (uc *UserController) ChangeEmail(c *gin.Context) {
	type Query struct {
		Email string `json:"email" binding:"required,email" example:"newmail@newcomp.com"` // New email address
	} //	@name	ChangeEmailQuery

	var query Query
	if !handle.JSONBind(c, &query) {
		return
	}
	id := c.GetUint("user_id")

	user, err := model.GetUserByID(uc.DB, id, "EmailChange")
	if err != nil {
		handle.ServerError(c, err)
		return
	}

	if user.Email == query.Email {
		handle.BadRequestError(c, "Email is the same as current email")
		return
	}

	if err = validate.Email(query.Email); err != nil {
		handle.BadRequestError(c, "Invalid email address")
		return
	}

	if user.EmailChange == nil {
		user.EmailChange = &model.UserEmailChange{UserID: id}
	}

	changeTokens, err := tokens.CreateResetTokens()
	if err != nil {
		handle.ServerError(c, err)
		return
	}
	user.EmailChange.NewEmail = query.Email
	user.EmailChange.ChangeTokenHash = changeTokens.TokenHash
	user.EmailChange.TokenExpiry = time.Now().Add(uc.ResetCfg.ExpirationTime)

	if err = uc.DB.Transaction(func(tx *gorm.DB) error {
		mailAvalialbe, dbErr := model.IsEmailAvailable(query.Email, tx, user.ID)
		if dbErr != nil {
			handle.ServerError(c, dbErr)
			return gorm.ErrInvalidTransaction
		}

		if !mailAvalialbe {
			handle.BadRequestError(c, "Email is already in use")
			return gorm.ErrInvalidTransaction
		}

		if err = tx.Save(user.EmailChange).Error; err != nil {
			handle.ServerError(c, err)
			return gorm.ErrInvalidTransaction
		}

		mailerErr := uc.Mailer.SendChangeEmail(
			"User",
			query.Email,
			changeTokens.Token,
			user.EmailChange.TokenExpiry,
		)
		if mailerErr != nil {
			handle.ServerError(c, mailerErr)
			return gorm.ErrInvalidTransaction
		}

		return nil
	}); err != nil {
		return
	}

	handle.Success(c, gin.H{"message": "Email change request token sent"})
}

// @Summary		Confirm email change for the user
// @Description	Confirms an email change for the user.
// @Description	The new email address will be active on the next login.
// @Description	You have to login (authenticate) with the old email address to confirm the change.
// @Tags			User
// @Produce		json
// @Param			request	body		usercontroller.ConfirmEmailChangeQuery			true	"Request body"
// @Success		200		{object}	handle.jsendSuccess[map[string]string]			"Email changed"
// @Failure		400		{object}	handle.jsendFailure[handle.errorResponse]		"Invalid token"
// @Failure		404		{object}	handle.jsendFailure[handle.errorResponse]		"No email change request found"
// @Failure		401		{object}	handle.jsendFailure[handle.errorResponse]		"Unauthorized"
// @Failure		403		{object}	handle.jsendFailure[handle.errorResponse]		"Token expired"
// @Failure		422		{object}	handle.jsendFailure[handle.validationResponse]	"Bad query format"
// @Failure		500		{object}	handle.jSendError								"Internal server error"
//
// @Security		Bearer
//
// @Router			/user/email/confirm [post]
func (uc *UserController) ConfirmEmailChange(c *gin.Context) {
	type Query struct {
		Token string `json:"token" binding:"required" example:"my_change_token"` // Change token
	} //	@name	ConfirmEmailChangeQuery

	var query Query
	if !handle.JSONBind(c, &query) {
		return
	}
	id := c.GetUint("user_id")

	user, err := model.GetUserByID(uc.DB, id, "EmailChange")
	if err != nil {
		handle.ServerError(c, err)
		return
	}

	if user.EmailChange == nil {
		handle.NotFoundError(c, "No email change request found")
		return
	}

	if validToken, _ := hash.Check(user.EmailChange.ChangeTokenHash, query.Token); !validToken {
		handle.BadRequestError(c, "Invalid token")
		return
	}

	if err = validate.TokenExpiry(user.EmailChange.TokenExpiry); err != nil {
		handle.ForbiddenError(c, "Token expired")
		return
	}

	user.Email = user.EmailChange.NewEmail
	if err = uc.DB.Transaction(func(tx *gorm.DB) error {
		if err = user.Save(tx); err != nil {
			handle.ServerError(c, err)
			return gorm.ErrInvalidTransaction
		}

		if err = tx.Unscoped().Delete(user.EmailChange).Error; err != nil {
			handle.ServerError(c, err)
			return gorm.ErrInvalidTransaction
		}

		return nil
	}); err != nil {
		return
	}

	handle.Success(c, gin.H{"message": "Email changed"})
}

// @Summary		Update user profile information
// @Description	Updates the user profile information. At least one field must be provided for update.
// @Description	The following fields can be updated: `first name`, `last name`, `organization`.
// @Tags			User
// @Produce		json
// @Param			request	body		usercontroller.UpdateProfileQuery				true	"Request body"
// @Success		200		{object}	handle.jsendSuccess[map[string]string]			"Profile updated"
// @Failure		401		{object}	handle.jsendFailure[handle.errorResponse]		"Unauthorized"
// @Failure		400		{object}	handle.jsendFailure[handle.errorResponse]		"No changes requested or invalid data"
// @Failure		422		{object}	handle.jsendFailure[handle.validationResponse]	"Bad query format"
// @Failure		500		{object}	handle.jSendError								"Internal server error"
//
// @Security		Bearer
//
// @Router			/user/profile [patch]
func (uc *UserController) UpdateProfile(c *gin.Context) {
	type Query struct {
		FirstName *string `json:"first_name,omitempty" binding:"omitempty,min=2,max=255" example:"Joe"`    // First name
		LastName  *string `json:"last_name,omitempty" binding:"omitempty,min=2,max=255" example:"Doe"`     // Last name
		Org       *string `json:"organization,omitempty" binding:"omitempty,min=2,max=255" example:"ACME"` // Organization
	} //	@name	UpdateProfileQuery

	var query Query
	if !handle.JSONBind(c, &query) {
		return
	}
	id := c.GetUint("user_id")

	if (query.FirstName == nil) && (query.LastName == nil) && (query.Org == nil) {
		handle.BadRequestError(c, "No data to update provided")
		return
	}

	user, err := model.GetUserByID(uc.DB, id)
	if err != nil {
		handle.ServerError(c, err)
		return
	}

	if err = helper.UpdateField(&user.FirstName, query.FirstName, validate.Name); err != nil {
		handle.BadRequestError(c, fmt.Sprintf("Invalid first name: %s", err))
		return
	}

	if err = helper.UpdateField(&user.LastName, query.LastName, validate.Name); err != nil {
		handle.BadRequestError(c, fmt.Sprintf("Invalid last name: %s", err))
		return
	}

	if err = helper.UpdateField(&user.Org, query.Org, validate.Organization); err != nil {
		handle.BadRequestError(c, fmt.Sprintf("Invalid organization name: %s", err))
		return
	}

	if err = user.Save(uc.DB); err != nil {
		handle.ServerError(c, err)
		return
	}

	handle.Success(c, gin.H{"message": "Profile updated"})
}

// @Summary	Get the user profile information
// @Tags		User
// @Produce	json
// @Success	200	{object}	handle.jsendSuccess[UserProfile]			"User profile"
// @Failure	401	{object}	handle.jsendFailure[handle.errorResponse]	"Unauthorized"
// @Failure	500	{object}	handle.jSendError							"Internal server error"
//
// @Security	Bearer
//
// @Router		/user/profile [get]
func (uc *UserController) GetProfile(c *gin.Context) {
	id := c.GetUint("user_id")

	user, err := model.GetUserByID(uc.DB, id)
	if err != nil {
		handle.ServerError(c, err)
		return
	}

	type UserProfile struct {
		Email     string     `json:"email" example:"joe@me.com"`                // Email address
		FirstName string     `json:"first_name" example:"Joe"`                  // First name
		LastName  string     `json:"last_name" example:"Doe"`                   // Last name
		Org       string     `json:"organization" example:"ACME"`               // Organization
		Role      string     `json:"role" example:"user"`                       // User role
		LastLogin *time.Time `json:"last_login" example:"2021-07-01T12:00:00Z"` // Last login time
	} //	@name	UserProfile

	userResult := UserProfile{
		Email:     user.Email,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Org:       user.Org,
		Role:      user.Role,
		LastLogin: user.LastLogin,
	}

	handle.Success(c, userResult)
}

// @Summary		Delete user account
// @Description	The account will be soft deleted.
// @Description	If the user is the last admin, the account cannot be deleted.
// @Description	If a user is soft-deleted, the account will be permanently deleted in the future.
// @Tags			User
// @Produce		json
// @Success		200	{object}	handle.jsendSuccess[map[string]string]		"Password reset"
// @Failure		401	{object}	handle.jsendFailure[handle.errorResponse]	"Unauthorized"
// @Failure		400	{object}	handle.jsendFailure[handle.errorResponse]	"Last admin account"
// @Failure		500	{object}	handle.jSendError							"Internal server error"
//
// @Security		Bearer
//
// @Router			/user [delete]
func (uc *UserController) DeleteAccount(c *gin.Context) {
	id := c.GetUint("user_id")

	user, err := model.GetUserByID(uc.DB, id)
	if err != nil {
		handle.ServerError(c, err)
		return
	}

	role := user.Role
	if err = uc.DB.Transaction(func(tx *gorm.DB) error {
		if role == "admin" {
			adminCount, errAdmin := model.CountActiveAdmins(tx)
			if errAdmin != nil {
				handle.ServerError(c, err)
				return gorm.ErrInvalidTransaction
			}

			if adminCount <= 1 {
				handle.BadRequestError(c, "Cannot delete the only admin account")
				return gorm.ErrInvalidTransaction
			}
		}

		if err = tx.Delete(&model.User{}, id).Error; err != nil {
			handle.ServerError(c, err)
			return err
		}

		return nil
	}); err != nil {
		return
	}

	handle.Success(c, gin.H{"message": "User deleted"})
}
