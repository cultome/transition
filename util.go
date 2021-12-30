package transition

import (
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func GetSchema(value interface{}, db *gorm.DB) *schema.Schema {
	stmt := &gorm.Statement{DB: db}
	stmt.Parse(&value)

	return stmt.Schema
}
