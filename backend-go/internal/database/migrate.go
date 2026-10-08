package database

import (
	"gorm.io/gorm"
)

func AutoMigrate(db *gorm.DB) error {
	models := []interface{}{
		&Admin{},
		&User{},
		&UserTraffic{},
		&Node{},
		&Host{},
		&ConfigProfile{},
		&ConfigProfileInbound{},
		&ConfigProfileInboundsToNodes{},
		&RemnawaveSetting{},
		&ApiToken{},
		&SubscriptionTemplate{},
		&SubscriptionPageConfig{},
		&SubscriptionSetting{},
		&ConfigProfileSnippet{},
		&SharedList{},
		&NodePlugin{},
		&InternalSquad{},
		&InternalSquadMember{},
		&InternalSquadInbound{},
		&ExternalSquadTemplate{},
		&ExternalSquad{},
		&InfraProvider{},
		&InfraBillingNode{},
		&InfraBillingHistory{},
		&HwidDevice{},
		&UserSubscriptionRequestHistory{},
		&EntityMeta{},
		&Keygen{},
		&Passkey{},
	}

	if db.Dialector.Name() == "postgres" {
		for _, m := range models {
			if !db.Migrator().HasTable(m) {
				if err := db.AutoMigrate(m); err != nil {
					return err
				}
			}
		}
	} else {
		if err := db.AutoMigrate(models...); err != nil {
			return err
		}
	}

	SeedDefaults(db)
	return nil
}
