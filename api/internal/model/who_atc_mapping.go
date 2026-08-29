package model

type WHOATCMapping struct {
	ATCCode    string  `gorm:"column:atc_code;primaryKey;size:16"`
	Level      int     `gorm:"column:level;not null"`
	LabelEN    *string `gorm:"column:label_en;type:varchar(255)"`
	SourceYear *int    `gorm:"column:source_year"`
	SourceURL  *string `gorm:"column:source_url;type:varchar(512)"`
}

func (WHOATCMapping) TableName() string {
	return "who_atc_mapping"
}
