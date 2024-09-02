package admincontroller

import (
	"errors"
	"fmt"
	"net/http"
	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/handle"
	"observeddb-go-api/internal/model"
	"observeddb-go-api/internal/utils/tokens"
	"observeddb-go-api/internal/utils/validate"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AdminController struct {
	DB       *gorm.DB
	ResetCfg cfg.ResetTokenConfig
}

func NewAdminController(resourceHandle *handle.ResourceHandle) *AdminController {
	return &AdminController{
		DB:       resourceHandle.Gorm,
		ResetCfg: resourceHandle.ResetCfg,
	}
}

func (ac *AdminController) CreateUser(c *gin.Context) {
	var query struct {
		Email     string `json:"email" binding:"required,email,min=2,max=255"`
		FirstName string `json:"first_name" binding:"required,min=2,max=255"`
		LastName  string `json:"last_name" binding:"required,min=2,max=255"`
		Org       string `json:"organization" binding:"required,min=2,max=255"`
		Role      string `json:"role" binding:"required,oneof=admin user approver"`
	}

	if !handle.JSONBind(c, &query) {
		return
	}

	if err := validate.Email(query.Email); err != nil {
		handle.BadRequestError(c, fmt.Sprintf("Invalid email: %s", err))
		return
	}

	if err := validate.Name(query.FirstName); err != nil {
		handle.BadRequestError(c, fmt.Sprintf("Invalid first name: %s", err))
		return
	}

	if err := validate.Name(query.LastName); err != nil {
		handle.BadRequestError(c, fmt.Sprintf("Invalid last name: %s", err))
		return
	}

	if err := validate.Organization(query.Org); err != nil {
		handle.BadRequestError(c, fmt.Sprintf("Invalid organization: %s", err))
		return
	}

	// create user
	user := model.User{
		Email:     query.Email,
		FirstName: query.FirstName,
		LastName:  query.LastName,
		Org:       query.Org,
		Role:      query.Role,
		PwdReset:  &model.UserPwdReset{},
	}
	resetTokens, err := tokens.CreateResetTokens()
	if err != nil {
		handle.ServerError(c, err)
		return
	}
	user.PwdReset.ResetTokenHash = resetTokens.TokenHash
	user.PwdReset.TokenExpiry = time.Now().Add(ac.ResetCfg.ExpirationTime)

	// check if email is available and create a user +
	if err = ac.DB.Transaction(func(tx *gorm.DB) error {
		mailAvailable, mailErr := model.IsEmailAvailable(query.Email, tx, 0)
		if mailErr != nil {
			handle.ServerError(c, mailErr)
			return gorm.ErrInvalidTransaction
		}

		if !mailAvailable {
			handle.BadRequestError(c, "Email already in use")
			return gorm.ErrInvalidTransaction
		}

		return tx.Create(&user).Error
	}); err != nil {
		return
	}

	if gin.IsDebugging() {
		c.JSON(http.StatusCreated, gin.H{"message": "User created", "token": resetTokens.Token})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "User created"})
}

func (ac *AdminController) GetUsers(c *gin.Context) {
	var query struct {
		Role   string `form:"role" binding:"omitempty,oneof=admin user approver"`
		Status string `form:"status" binding:"omitempty,oneof=active inactive"`
	}

	if !handle.QueryBind(c, &query) {
		return
	}

	db := ac.DB
	if query.Role != "" {
		db = db.Where(&model.User{Role: query.Role})
	}

	if query.Status != "" {
		db = db.Where(&model.User{Status: query.Status})
	}

	var users []model.User
	if err := db.Find(&users).Error; err != nil {
		handle.ServerError(c, err)
		return
	}

	if len(users) == 0 {
		handle.NotFoundError(c, "No users found that match the query")
		return
	}

	c.JSON(http.StatusOK, users)
}

func (ac *AdminController) GetUserByEmail(c *gin.Context) {
	user, err := model.GetUserByEmail(ac.DB, c.Param("email"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			handle.NotFoundError(c, "User not found")
			return
		}

		handle.ServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, user)
}

func (ac *AdminController) DeleteUserByEmail(c *gin.Context) {
	emailToDelete := c.Param("email")
	adminEmail := c.GetString("user_email")

	if emailToDelete == adminEmail {
		handle.ForbiddenError(c, "Cannot delete own account")
		return
	}

	user, err := model.GetUserByEmail(ac.DB, emailToDelete)
	if err != nil {
		handle.NotFoundError(c, "User not found")
		return
	}

	if err = ac.DB.Delete(&user).Error; err != nil {
		handle.ServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deleted"})
}

func (ac *AdminController) ChangeUserProfile(c *gin.Context) {
	var query struct {
		Role   string `json:"role" binding:"omitempty,oneof=admin user approver"`
		Status string `json:"status" binding:"omitempty,oneof=active inactive"`
	}
	adminID := c.GetUint("user_id")

	if !handle.JSONBind(c, &query) {
		return
	}

	user, err := model.GetUserByEmail(ac.DB, c.Param("email"))
	if err != nil {
		handle.NotFoundError(c, "User not found")
		return
	}

	if query.Role == "" && query.Status == "" {
		handle.BadRequestError(c, "No changes requested")
		return
	}

	if user.ID == adminID {
		adminCount, adminErr := model.CountActiveAdmins(ac.DB)
		if adminErr != nil {
			handle.ServerError(c, err)
			return
		}

		if adminCount == 1 {
			handle.ForbiddenError(c, "Cannot change admin's role or status (only one admin left)")
			return
		}

		handle.ForbiddenError(c, "Cannot change own role or status")
		return
	}

	if query.Role != "" {
		user.Role = query.Role
	}

	if query.Status != "" {
		user.Status = query.Status
	}

	if err = ac.DB.Save(&user).Error; err != nil {
		handle.ServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User profile updated"})
}
