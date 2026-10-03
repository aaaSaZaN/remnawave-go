package database

import (
	"gorm.io/gorm"
)

func AutoMigrate(db *gorm.DB) error {
	err := db.AutoMigrate(
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
		&ExternalSquad{},
		&InfraProvider{},
		&InfraBillingNode{},
		&InfraBillingHistory{},
		&HwidDevice{},
		&UserSubscriptionRequestHistory{},
		&EntityMeta{},
		&Keygen{},
		&Passkey{},
	)
	if err != nil {
		return err
	}

	SeedDefaults(db)
	return nil
}
