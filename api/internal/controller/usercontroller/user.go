package usercontroller

import (
	"fmt"
	"net/http"
	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/handle"
	"observeddb-go-api/internal/model"
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
}

func NewUserController(resourceHandle *handle.ResourceHandle) *UserController {
	return &UserController{
		DB:       resourceHandle.Gorm,
		AuthCfg:  resourceHandle.AuthCfg,
		ResetCfg: resourceHandle.ResetCfg,
	}
}

type loginResponse struct {
	tokens.AuthTokens
	Role      string     `json:"role"`
	LastLogin *time.Time `json:"last_login"`
}

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

func (uc *UserController) Login(c *gin.Context) {
	var query struct {
		Login    string  `json:"login" binding:"required"`
		Password string  `json:"password" binding:"required"`
		Role     *string `json:"role" binding:"omitempty,oneof=admin user approver"`
	}

	if !handle.JSONBind(c, &query) {
		return
	}

	user, err := model.GetUserByEmail(uc.DB, query.Login)
	if err != nil {
		handle.UnauthorizedError(c)
		return
	}

	if user.PwdHash == nil {
		handle.UnauthorizedError(c)
		return
	}

	if validPwd, _ := hash.Check(*user.PwdHash, query.Password); !validPwd {
		handle.UnauthorizedError(c)
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
	c.JSON(http.StatusOK, res)
}

func (uc *UserController) RefreshToken(c *gin.Context) {
	var query struct {
		Token string `json:"refresh_token" binding:"required"`
	}

	if !handle.JSONBind(c, &query) {
		return
	}

	claims, err := tokens.CheckRefreshToken(query.Token, &uc.AuthCfg)
	if err != nil {
		handle.UnauthorizedError(c)
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

	result := newLoginResponse(newToken, user.Role, user.LastLogin)
	_ = user.UpdateLastLogin(uc.DB)
	c.JSON(http.StatusOK, result)
}

func (uc *UserController) ChangePwd(c *gin.Context) {
	var query struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}

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

	c.JSON(http.StatusOK, gin.H{"message": "Password changed"})
}

func (uc *UserController) ResetPwd(c *gin.Context) {
	var query struct {
		Email string `json:"email" binding:"required,email"`
	}

	if !handle.JSONBind(c, &query) {
		return
	}

	// We don't want to leak information about registered emails so we always return the same message
	const defaultMsg = "If your email is registered, you will receive a password reset link."
	user, err := model.GetUserByEmail(uc.DB, query.Email, "PwdReset")
	if err != nil {
		c.JSON(http.StatusAccepted, gin.H{"message": defaultMsg})
		return
	}

	if user.Status != "active" {
		c.JSON(http.StatusAccepted, gin.H{"message": defaultMsg})
		return
	}

	if user.PwdReset != nil {
		if err = validate.QueryRetry(user.PwdReset.UpdatedAt, uc.ResetCfg.RetryInterval); err != nil {
			c.JSON(http.StatusAccepted, gin.H{"message": defaultMsg})
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

	err = uc.DB.Save(user.PwdReset).Error
	if err != nil {
		handle.ServerError(c, err)
		return
	}

	if gin.IsDebugging() {
		c.JSON(http.StatusOK, gin.H{"message": defaultMsg, "token": resetTokens.Token})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"message": defaultMsg})
}

func (uc *UserController) ResetPwdConfirm(c *gin.Context) {
	var query struct {
		Token    string `json:"token" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required"`
	}

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
		handle.BadRequestError(c, "Token expired")
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

	c.JSON(http.StatusOK, gin.H{"message": "Password reset"})
}

func (uc *UserController) ChangeEmail(c *gin.Context) {
	var query struct {
		Email string `json:"email" binding:"required,email"`
	}

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
		handle.BadRequestError(c, "Invalid email")
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

		return nil
	}); err != nil {
		return
	}

	if gin.IsDebugging() {
		c.JSON(http.StatusOK, gin.H{"message": "Email change request sent", "token": changeTokens.Token})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Email change request sent"})
}

func (uc *UserController) ConfirmEmailChange(c *gin.Context) {
	var query struct {
		Token string `json:"token" binding:"required"`
	}

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

	c.JSON(http.StatusOK, gin.H{"message": "Email changed"})
}

func (uc *UserController) UpdateProfile(c *gin.Context) {
	var query struct {
		FirstName *string `json:"first_name,omitempty" binding:"omitempty,min=2,max=255"`
		LastName  *string `json:"last_name,omitempty" binding:"omitempty,min=2,max=255"`
		Org       *string `json:"organization,omitempty" binding:"omitempty,min=2,max=255"`
	}

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

	c.JSON(http.StatusOK, gin.H{"message": "Profile updated"})
}

func (uc *UserController) GetProfile(c *gin.Context) {
	id := c.GetUint("user_id")

	user, err := model.GetUserByID(uc.DB, id)
	if err != nil {
		handle.ServerError(c, err)
		return
	}

	userResult := struct {
		Email     string     `json:"email"`
		FirstName string     `json:"first_name"`
		LastName  string     `json:"last_name"`
		Org       string     `json:"organization"`
		Role      string     `json:"role"`
		LastLogin *time.Time `json:"last_login"`
	}{
		Email:     user.Email,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Org:       user.Org,
		Role:      user.Role,
		LastLogin: user.LastLogin,
	}

	c.JSON(http.StatusOK, userResult)
}

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

	c.JSON(http.StatusOK, gin.H{"message": "User deleted"})
}
