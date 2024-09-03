package server

import (
	"observeddb-go-api/internal/controller/admincontroller"
	"observeddb-go-api/internal/controller/formulationcontroller"
	"observeddb-go-api/internal/controller/interactioncontroller"
	"observeddb-go-api/internal/controller/syscontroller"
	"observeddb-go-api/internal/controller/usercontroller"
	"observeddb-go-api/internal/handle"
	"observeddb-go-api/internal/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterSysRoutes(r *gin.RouterGroup, resourceHandle *handle.ResourceHandle) {
	c := syscontroller.NewSysController(resourceHandle)

	users := r.Group("/sys")
	{
		users.GET("/ping", c.GetPing)
		users.GET("/info", c.GetInfo)
	}
}

func RegisterUserRoutes(r *gin.RouterGroup, resourceHandle *handle.ResourceHandle) {
	c := usercontroller.NewUserController(resourceHandle)

	// no auth here
	user := r.Group("/user")
	{
		user.POST("/login", c.Login)
		user.POST("/refresh-token", c.RefreshToken)
		user.POST("/password/reset", c.ResetPwd)
		user.POST("/password/init", c.ResetPwd)
		user.POST("/password/reset/confirm", c.ResetPwdConfirm)
	}

	authUser := r.Group("/user")
	authUser.Use(middleware.Authentication(&resourceHandle.AuthCfg))
	{
		authUser.PATCH("/password", c.ChangePwd)
		authUser.PATCH("/email", c.ChangeEmail)
		authUser.POST("/email/confirm", c.ConfirmEmailChange)
		authUser.DELETE("/", c.DeleteAccount)
		authUser.GET("/profile", c.GetProfile)
		authUser.PATCH("/profile", c.UpdateProfile)
	}
}

func RegisterAdminRoutes(r *gin.RouterGroup, resourceHandle *handle.ResourceHandle) {
	c := admincontroller.NewAdminController(resourceHandle)

	authUser := r.Group("/admin")
	authUser.Use(middleware.Authentication(&resourceHandle.AuthCfg), middleware.AdminAccess())
	{
		authUser.POST("/users/", c.CreateUser)
		authUser.GET("/users", c.GetUsers)
		authUser.GET("/users/:email", c.GetUserByEmail)
		authUser.DELETE("/users/:email", c.DeleteUserByEmail)
		authUser.PATCH("/users/:email", c.ChangeUserProfile)
	}
}

func RegisterFormulationRoutes(r *gin.RouterGroup, resourceHandle *handle.ResourceHandle) {
	c := formulationcontroller.NewFormulationController(resourceHandle)

	profiles := r.Group("/formulations")
	{
		profiles.GET("/", c.GetFormulations)
	}
}

func RegisterInteractionRoutes(r *gin.RouterGroup, resourceHandle *handle.ResourceHandle) {
	c := interactioncontroller.NewInteractionController(resourceHandle)

	profiles := r.Group("/interactions")
	{
		profiles.GET("/description", c.GetInterDescription)
		profiles.GET("/pzns", c.GetInterPZNs)
		profiles.POST("/pzns", c.PostInterPZNs)
		profiles.GET("/compounds", c.GetInterCompounds)
		profiles.POST("/compounds", c.PostInterCompounds)
	}
}
