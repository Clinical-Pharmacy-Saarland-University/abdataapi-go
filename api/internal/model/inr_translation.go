package model

import "time"

type INRTranslation struct {
	KeyIND            string     `gorm:"column:key_ind;primaryKey;size:16"`
	Counter           int        `gorm:"column:zaehler;primaryKey"`
	NameDE            string     `gorm:"column:name_de;type:varchar(500);not null"`
	NameEN            string     `gorm:"column:name_en;type:varchar(500);not null"`
	TranslationSource string     `gorm:"column:translation_source;type:varchar(64);not null"`
	TranslationDate   *time.Time `gorm:"column:translation_date;type:date"`
	ConfidenceLevel   string     `gorm:"column:confidence_level;type:varchar(32);not null"`
	ValidationStatus  string     `gorm:"column:validation_status;type:varchar(64);not null;index"`
	ReviewSource      *string    `gorm:"column:review_source;type:varchar(64)"`
	ReviewDate        *time.Time `gorm:"column:review_date;type:date"`
	ReviewedNameEN    *string    `gorm:"column:reviewed_name_en;type:varchar(500)"`
	ReviewNotes       *string    `gorm:"column:review_notes;type:text"`
}

func (INRTranslation) TableName() string {
	return "TRANSLATION_INR_C"
}
