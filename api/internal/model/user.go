package model

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

type User struct {
	gorm.Model `json:"-"`
	Email      string     `gorm:"index:idx_email_deleted_at,unique;size:255;not null" json:"email"`
	LastName   string     `gorm:"not null;size:255" json:"last_name"`
	FirstName  string     `gorm:"not null;size:255" json:"first_name"`
	Org        string     `gorm:"not null;size:255" json:"organization"`
	Role       string     `gorm:"type:enum('admin','user','approver');not null" json:"role"`
	Status     string     `gorm:"type:enum('active','inactive');default:'active';not null" json:"status"`
	LastLogin  *time.Time `gorm:"type:timestamp;" json:"last_login"`
	PwdHash    *string    `gorm:"default:null;size:255" json:"-"`
	// Soft delete
	DeletedAt   gorm.DeletedAt   `gorm:"index:idx_email_deleted_at,unique" json:"-"`
	PwdReset    *UserPwdReset    `gorm:"foreignKey:UserID" json:"-"`
	EmailChange *UserEmailChange `gorm:"foreignKey:UserID" json:"-"`
}

type UserPwdReset struct {
	gorm.Model
	ResetTokenHash string    `gorm:"size:255;not null"`
	TokenExpiry    time.Time `gorm:"type:timestamp;not null"`
	UserID         uint      `gorm:"index; not null"`
}

type UserEmailChange struct {
	gorm.Model
	NewEmail        string    `gorm:"unique;size:255;not null"`
	ChangeTokenHash string    `gorm:"size:255;not null"`
	TokenExpiry     time.Time `gorm:"type:timestamp;not null"`
	UserID          uint      `gorm:"index; not null"`
}

// Delete associated records after deleting a user (soft delete included).
func (u *User) AfterDelete(tx *gorm.DB) error {
	if err := tx.Unscoped().Where("user_id = ?", u.ID).Delete(&UserPwdReset{}).Error; err != nil {
		return err
	}

	if err := tx.Unscoped().Where("user_id = ?", u.ID).Delete(&UserEmailChange{}).Error; err != nil {
		return err
	}

	return nil
}

func (u User) MarshalJSON() ([]byte, error) {
	type Alias User

	aux := struct {
		Alias
		PwdHashSet bool `json:"password_set"`
	}{
		Alias:      Alias(u),
		PwdHashSet: u.PwdHash != nil,
	}

	return json.Marshal(&aux) //nolint: wrapcheck // no need to wrap
}

func (u *User) Save(db *gorm.DB) error {
	return db.Save(u).Error
}

func (u *User) UpdateLastLogin(db *gorm.DB) error {
	now := time.Now()
	return db.Model(u).Updates(User{LastLogin: &now}).Error
}

func CountActiveAdmins(db *gorm.DB) (int64, error) {
	var count int64
	if err := db.Model(&User{}).Where(&User{Role: "admin", Status: "active"}).Count(&count).Error; err != nil {
		return 0, err
	}

	return count, nil
}

func GetUserByEmail(db *gorm.DB, email string, joins ...string) (*User, error) {
	for _, join := range joins {
		db = db.Joins(join)
	}

	var user User
	if err := db.Where(&User{Email: email}).First(&user).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

func GetUserByID(db *gorm.DB, id uint, joins ...string) (*User, error) {
	for _, join := range joins {
		db = db.Joins(join)
	}

	var user User
	if err := db.First(&user, id).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

func IsEmailAvailable(mail string, tx *gorm.DB, excludeID uint) (bool, error) {
	var exists bool

	userQuery := tx.Model(&User{}).Select("1").Where(&User{Email: mail})
	if excludeID != 0 {
		userQuery = userQuery.Where("id != ?", excludeID)
	}
	if err := userQuery.Limit(1).Find(&exists).Error; err != nil || exists {
		return false, err
	}

	emailChangeQuery := tx.Model(&UserEmailChange{}).Select("1").Where(&UserEmailChange{NewEmail: mail})
	if excludeID != 0 {
		emailChangeQuery = emailChangeQuery.Where("user_id != ?", excludeID)
	}
	if err := emailChangeQuery.Limit(1).Find(&exists).Error; err != nil || exists {
		return false, err
	}

	return true, nil
}
