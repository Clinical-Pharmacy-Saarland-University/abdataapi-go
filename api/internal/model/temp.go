package model

import (
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Specimen struct {
	gorm.Model
	Name      string   `gorm:"type:varchar(255);not null" json:"name" binding:"required"`
	Category  string   `gorm:"type:enum('small', 'large', 'pd', 'covariate', 'other');not null" json:"category" binding:"required"`
	PubchemID *int     `gorm:"type:int;unique;" json:"pubchem_id"`
	MW        *float64 `gorm:"type:double" json:"mw"`
	Comment   *string  `gorm:"type:text" json:"comment"`
}

type Reference struct {
	gorm.Model
	UUID        string  `gorm:"type:char(36);unique;not null" json:"uuid"`
	Key         string  `gorm:"type:varchar(255);not null;unique" json:"key" binding:"required"`
	FirstAuthor *string `gorm:"type:text" json:"first_author"`
	DOI         *string `gorm:"type:varchar(255);unique" json:"doi"`
	PMID        *int    `gorm:"type:int;unique" json:"pmid"`
	AltID       *string `gorm:"type:varchar(255)" json:"alt_id"`
	URL         *string `gorm:"type:varchar(255)" json:"url"`
	Year        *int    `gorm:"type:int" json:"year"`

	Datasets []*Dataset `gorm:"many2many:data_references" json:"datasets"`
}

func (r *Reference) BeforeCreate(_ *gorm.DB) error {
	r.UUID = uuid.New().String()
	return nil
}

type Dataset struct {
	gorm.Model
	UUID string `gorm:"type:char(36);unique;not null" json:"uuid"`
	Name string `gorm:"type:varchar(255);not null" json:"name" binding:"required"`

	Profiles   []Profile   `gorm:"foreignKey:DatasetID" json:"profiles" binding:"required"`
	References []Reference `gorm:"many2many:data_references" json:"references" binding:"required"`
}

func (ds *Dataset) BeforeCreate(_ *gorm.DB) error {
	ds.UUID = uuid.New().String()
	return nil
}

type Profile struct {
	gorm.Model
	UUID        string  `gorm:"type:char(36);unique;not null" json:"uuid"`
	Type        string  `gorm:"type:enum('typical', 'aggregated', 'individual');not null" json:"type" binding:"required"`
	Source      *string `gorm:"type:varchar(255)" json:"source"`
	ClockTime   *string `gorm:"type:varchar(10)" json:"clock_time"`
	Organ       *string `gorm:"type:varchar(255)" json:"organ"`
	Compartment *string `gorm:"type:varchar(255)" json:"compartment"`
	Matrix      *string `gorm:"type:varchar(255)" json:"matrix"`

	DatasetID     uint `gorm:"index; not null"`
	SpecimenID    uint `gorm:"index; not null"`
	ObservationID uint `gorm:"index"`

	Specimen    Specimen     `gorm:"foreignKey:SpecimenID" json:"specimen" binding:"required"`
	Observation *Observation `gorm:"foreignKey:ObservationID" json:"observation"`
}

func (p *Profile) BeforeCreate(_ *gorm.DB) error {
	p.UUID = uuid.New().String()
	return nil
}

type Dataframe struct {
	Time []float64 `json:"time"`
	Data []float64 `json:"data"`
	Var  []float64 `json:"var"`
}

type Observation struct {
	gorm.Model
	UUID      string          `gorm:"type:char(36);unique;not null" json:"uuid"`
	Dataframe json.RawMessage `gorm:"type:json;not null" json:"dataframe" binding:"required"`
	TimeUnit  string          `gorm:"type:varchar(50);not null" json:"time_unit" binding:"required"`
	ValueUnit string          `gorm:"type:varchar(50);not null" json:"value_unit" binding:"required"`
	VarType   string          `gorm:"type:varchar(50);not null" json:"var_type" binding:"required"`
	VarUnit   string          `gorm:"type:varchar(50);not null" json:"var_unit" binding:"required"`
	Comment   *string         `gorm:"type:text" json:"comment"`

	ProfileID uint `gorm:"index; not null"`
}

func (o *Observation) BeforeCreate(_ *gorm.DB) error {
	o.UUID = uuid.New().String()
	return nil
}

// /////////////////////////////////////////
// ////////////////////////////////////////
// ///////////////////////////////////////
// type Dataset struct {
// 	gorm.Model
// 	UUID        string    `gorm:"type:uuid;unique;not null"`
// 	ReferenceID uint      `gorm:"not null"` // Foreign key to Reference
// 	IsApproved  bool      `gorm:"default:false"`
// 	Profiles    []Profile `gorm:"foreignKey:DatasetID"`
// 	SubmittedBy uint      `gorm:"not null"` // Foreign key to Users
// }

// type DatasetVersion struct {
// 	gorm.Model
// 	DatasetUUID   string    `gorm:"type:uuid;not null"` // Reference to Dataset UUID
// 	VersionNumber uint      `gorm:"not null"`
// 	Data          string    `gorm:"type:jsonb;not null"` // Store the entire dataset as JSON
// 	ChangedAt     time.Time `gorm:"not null"`
// 	ChangedBy     uint      `gorm:"not null"`                  // Foreign key to Users
// 	Action        string    `gorm:"type:varchar(50);not null"` // Enum: 'Create', 'Update', 'Delete', 'Approve', etc.
// }

// type Demographic struct {
// 	gorm.Model
// 	Age       int       `gorm:"not null"`
// 	Gender    string    `gorm:"type:varchar(10)"`
// 	Ethnicity string    `gorm:"type:varchar(50)"`
// 	Weight    float64   `gorm:"not null"`
// 	Height    float64   `gorm:"not null"`
// 	Genetics  string    `gorm:"type:text"` // JSON or text field for genetics information
// 	Profiles  []Profile `gorm:"foreignKey:DemographicID"`
// }

// type AdministrationProtocol struct {
// 	gorm.Model
// 	Description string    `gorm:"type:text;not null"`
// 	Timepoints  string    `gorm:"type:text"` // JSON or array of timepoints
// 	Profiles    []Profile `gorm:"foreignKey:AdministrationProtocolID"`
// }

// type MealProtocol struct {
// 	gorm.Model
// 	Description     string    `gorm:"type:text;not null"`
// 	MealComposition string    `gorm:"type:text"` // JSON or array to describe meal components
// 	Profiles        []Profile `gorm:"foreignKey:MealProtocolID"`
// }

// type DerivedValue struct {
// 	gorm.Model
// 	ProfileID    uint    `gorm:"not null"`           // Foreign key to Profile
// 	AUC          float64 `gorm:"type:decimal(10,2)"` // Area Under the Curve
// 	Cmax         float64 `gorm:"type:decimal(10,2)"` // Maximum concentration
// 	OtherMetrics string  `gorm:"type:text"`          // JSON or additional columns for other derived values
// }

// type Interaction struct {
// 	gorm.Model
// 	InteractionType string    `gorm:"type:varchar(50);not null"` // Enum: 'Drug-Drug', 'Drug-Gene', 'Drug-Food', etc.
// 	Description     string    `gorm:"type:text"`
// 	RatioAUC        float64   `gorm:"type:decimal(10,2);null"`
// 	RatioCmax       float64   `gorm:"type:decimal(10,2);null"`
// 	Profiles        []Profile `gorm:"many2many:interaction_profiles;"` // Many-to-many relationship with Profile
// }

// type DatasetVersion struct {
// 	gorm.Model
// 	DatasetUUID   string    `gorm:"type:uuid;not null"` // Reference to Dataset UUID
// 	VersionNumber uint      `gorm:"not null"`
// 	Data          string    `gorm:"type:jsonb;not null"` // Store the entire dataset as JSON
// 	ChangedAt     time.Time `gorm:"not null"`
// 	ChangedBy     uint      `gorm:"not null"`                  // Foreign key to Users
// 	Action        string    `gorm:"type:varchar(50);not null"` // Enum: 'Create', 'Update', 'Delete', 'Approve', etc.
// }
