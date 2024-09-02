package database

import (
	"fmt"
	"observeddb-go-api/internal/model"
	"observeddb-go-api/internal/utils/hash"
	"observeddb-go-api/internal/utils/validate"
	"os"

	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) error {
	db.Set("gorm:table_options", "ENGINE=InnoDB")

	if err := db.AutoMigrate(&model.User{}, &model.UserEmailChange{}, &model.UserPwdReset{}); err != nil {
		return fmt.Errorf("cannot migrate user models: %w", err)
	}

	// Seed database with default admin user if no active admin user exists
	if err := seed(db); err != nil {
		return fmt.Errorf("cannot seed database: %w", err)
	}

	return nil
}

func seed(db *gorm.DB) error {
	count, err := model.CountActiveAdmins(db)
	if err != nil {
		return fmt.Errorf("cannot count active admins: %w", err)
	}

	if count == 0 {
		adminEmail := os.Getenv("ADMIN_EMAIL")
		adminPWD := os.Getenv("ADMIN_PASSWORD")

		if err = seedAdminUser(db, adminEmail, adminPWD); err != nil {
			return fmt.Errorf("cannot seed default admin: %w", err)
		}
	}

	return nil
}

func seedAdminUser(db *gorm.DB, email string, pwd string) error {
	if err := validate.Email(email); err != nil {
		return fmt.Errorf("invalid admin email: %w", err)
	}

	if err := validate.Password(pwd); err != nil {
		return fmt.Errorf("invalid admin password: %w", err)
	}

	pwd, err := hash.Create(pwd)
	if err != nil {
		return fmt.Errorf("cannot hash password: %w", err)
	}

	user := &model.User{
		Email:     email,
		LastName:  "Admin",
		FirstName: "Admin",
		Org:       "Admin",
		Role:      "admin",
		Status:    "active",
		PwdHash:   &pwd,
	}

	if err = user.Save(db); err != nil {
		return fmt.Errorf("cannot create admin user: %w", err)
	}

	return nil
}
