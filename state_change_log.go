package transition

import (
	"github.com/qor/admin"
	"github.com/qor/audited"
	"github.com/qor/qor/resource"
	"github.com/qor/roles"
	"gorm.io/gorm"
)

// StateChangeLog a model that used to keep state change logs
type StateChangeLog struct {
	gorm.Model
	audited.AuditedModel
	ID        string `sql:"type:uuid;primary_key" gorm:"default:uuid_generate_v4()"`
	ReferType string
	ReferID   string
	From      string
	To        string
	Note      string `sql:"size:1024"`
}

// GetStateChangeLogs get state change logs
func GetStateChangeLogs(model Stater, db *gorm.DB) []StateChangeLog {
	var (
		changelogs []StateChangeLog
		schema     = GetSchema(model, db)
	)

	db.Where("refer_type = ? AND refer_id = ?", schema.Table, model.RecordID()).Find(&changelogs)

	return changelogs
}

// GetLastStateChange gets last state change
func GetLastStateChange(model Stater, db *gorm.DB) *StateChangeLog {
	var (
		changelog StateChangeLog
		schema    = GetSchema(model, db)
	)

	db.Where("refer_type = ? AND refer_id = ?", schema.Table, model.RecordID()).Last(&changelog)
	if changelog.To == "" {
		return nil
	}
	return &changelog
}

// ConfigureQorResource used to configure transition for qor admin
func (stageChangeLog *StateChangeLog) ConfigureQorResource(res resource.Resourcer) {
	if res, ok := res.(*admin.Resource); ok {
		if res.Permission == nil {
			res.Permission = roles.Deny(roles.Update, roles.Anyone).Deny(roles.Create, roles.Anyone)
		} else {
			res.Permission = res.Permission.Deny(roles.Update, roles.Anyone).Deny(roles.Create, roles.Anyone)
		}
	}
}
