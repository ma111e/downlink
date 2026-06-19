package models

// DigestGlossary records which global glossary entries a digest references.
type DigestGlossary struct {
	DigestId string         `gorm:"primaryKey;index" json:"digest_id"`
	EntryId  string         `gorm:"primaryKey" json:"entry_id"`
	Entry    *GlossaryEntry `gorm:"foreignKey:EntryId;references:Id" json:"entry,omitempty"`
}

func (DigestGlossary) TableName() string {
	return "digest_glossary"
}
