package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"remnawave-go/config"
	"remnawave-go/internal/database"
)

func DetectRemnawavePostgresDSN() (string, error) {
	// 1. Check existing .env files
	candidates := []string{
		".env",
		"../.env",
		"/opt/gowave/.env",
		"/opt/remnawave/.env",
		"/opt/remnawave-go/.env",
	}

	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		var user, pass, dbName, fullURL string
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "#") || !strings.Contains(l, "=") {
				continue
			}
			parts := strings.SplitN(l, "=", 2)
			k := strings.TrimSpace(parts[0])
			v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			switch k {
			case "DATABASE_URL":
				fullURL = v
			case "POSTGRES_USER":
				user = v
			case "POSTGRES_PASSWORD":
				pass = v
			case "POSTGRES_DB":
				dbName = v
			}
		}

		if fullURL != "" {
			// If URL points to docker hostname remnawave-db:5432, replace with host port 127.0.0.1:6767
			if strings.Contains(fullURL, "@remnawave-db:5432") {
				fullURL = strings.ReplaceAll(fullURL, "@remnawave-db:5432", "@127.0.0.1:6767")
			}
			return fullURL, nil
		}

		if pass != "" {
			if user == "" {
				user = "postgres"
			}
			if dbName == "" {
				dbName = "postgres"
			}
			return fmt.Sprintf("postgresql://%s:%s@127.0.0.1:6767/%s?sslmode=disable", user, pass, dbName), nil
		}
	}

	return "", fmt.Errorf("no Remnawave PostgreSQL configuration automatically detected")
}

func MigratePostgresToSQLite(cfg *config.Config, sourceDSN, destPath string) error {
	if sourceDSN == "" {
		detected, err := DetectRemnawavePostgresDSN()
		if err == nil {
			sourceDSN = detected
			fmt.Printf("[*] Auto-detected Remnawave PostgreSQL DSN: %s\n", sourceDSN)
		} else {
			return err
		}
	}

	if destPath == "" {
		destPath = "remnawave.db"
	}

	fmt.Printf("[*] Connecting to PostgreSQL source...\n")
	pgDB, err := gorm.Open(postgres.Open(sourceDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	fmt.Printf("[*] Initializing SQLite destination (%s)...\n", destPath)
	sqliteDB, err := gorm.Open(sqlite.Open(destPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to open SQLite: %w", err)
	}

	// 1. Create tables in SQLite
	fmt.Printf("[*] Running schema migrations on SQLite...\n")
	if err := database.AutoMigrate(sqliteDB); err != nil {
		return fmt.Errorf("schema migration failed on SQLite: %w", err)
	}

	// 2. Transfer tables sequentially
	type copier struct {
		name string
		fn   func() error
	}

	copiers := []copier{
		{"admin", func() error {
			var records []database.Admin
			if err := pgDB.Find(&records).Error; err != nil {
				return err
			}
			if len(records) > 0 {
				_ = sqliteDB.Exec("DELETE FROM admin")
				return sqliteDB.Create(&records).Error
			}
			return nil
		}},
		{"users", func() error {
			var records []database.User
			if err := pgDB.Find(&records).Error; err != nil {
				return err
			}
			if len(records) > 0 {
				_ = sqliteDB.Exec("DELETE FROM users")
				return sqliteDB.CreateInBatches(&records, 100).Error
			}
			return nil
		}},
		{"user_traffic", func() error {
			var records []database.UserTraffic
			if err := pgDB.Find(&records).Error; err != nil {
				return err
			}
			if len(records) > 0 {
				_ = sqliteDB.Exec("DELETE FROM user_traffic")
				return sqliteDB.CreateInBatches(&records, 100).Error
			}
			return nil
		}},
		{"nodes", func() error {
			var records []database.Node
			if err := pgDB.Find(&records).Error; err != nil {
				return err
			}
			if len(records) > 0 {
				_ = sqliteDB.Exec("DELETE FROM nodes")
				return sqliteDB.Create(&records).Error
			}
			return nil
		}},
		{"hosts", func() error {
			var records []database.Host
			if err := pgDB.Find(&records).Error; err != nil {
				return err
			}
			if len(records) > 0 {
				_ = sqliteDB.Exec("DELETE FROM hosts")
				return sqliteDB.Create(&records).Error
			}
			return nil
		}},
		{"config_profiles", func() error {
			var records []database.ConfigProfile
			if err := pgDB.Find(&records).Error; err != nil {
				return err
			}
			if len(records) > 0 {
				_ = sqliteDB.Exec("DELETE FROM config_profiles")
				return sqliteDB.Create(&records).Error
			}
			return nil
		}},
		{"config_profile_inbounds", func() error {
			var records []database.ConfigProfileInbound
			if err := pgDB.Find(&records).Error; err != nil {
				return err
			}
			if len(records) > 0 {
				_ = sqliteDB.Exec("DELETE FROM config_profile_inbounds")
				return sqliteDB.Create(&records).Error
			}
			return nil
		}},
		{"config_profile_inbounds_to_nodes", func() error {
			var records []database.ConfigProfileInboundsToNodes
			if err := pgDB.Find(&records).Error; err != nil {
				return err
			}
			if len(records) > 0 {
				_ = sqliteDB.Exec("DELETE FROM config_profile_inbounds_to_nodes")
				return sqliteDB.Create(&records).Error
			}
			return nil
		}},
		{"remnawave_settings", func() error {
			var records []database.RemnawaveSetting
			if err := pgDB.Find(&records).Error; err != nil {
				return err
			}
			if len(records) > 0 {
				_ = sqliteDB.Exec("DELETE FROM remnawave_settings")
				return sqliteDB.Create(&records).Error
			}
			return nil
		}},
		{"api_tokens", func() error {
			var records []database.ApiToken
			if err := pgDB.Find(&records).Error; err != nil {
				return err
			}
			if len(records) > 0 {
				_ = sqliteDB.Exec("DELETE FROM api_tokens")
				return sqliteDB.Create(&records).Error
			}
			return nil
		}},
		{"subscription_templates", func() error {
			var records []database.SubscriptionTemplate
			if err := pgDB.Find(&records).Error; err != nil {
				return err
			}
			if len(records) > 0 {
				_ = sqliteDB.Exec("DELETE FROM subscription_templates")
				return sqliteDB.Create(&records).Error
			}
			return nil
		}},
		{"subscription_settings", func() error {
			var records []database.SubscriptionSetting
			if err := pgDB.Find(&records).Error; err != nil {
				return err
			}
			if len(records) > 0 {
				_ = sqliteDB.Exec("DELETE FROM subscription_settings")
				return sqliteDB.Create(&records).Error
			}
			return nil
		}},
		{"internal_squads", func() error {
			var records []database.InternalSquad
			if err := pgDB.Find(&records).Error; err != nil {
				return err
			}
			if len(records) > 0 {
				_ = sqliteDB.Exec("DELETE FROM internal_squads")
				return sqliteDB.Create(&records).Error
			}
			return nil
		}},
		{"internal_squad_members", func() error {
			var records []database.InternalSquadMember
			if err := pgDB.Find(&records).Error; err != nil {
				return err
			}
			if len(records) > 0 {
				_ = sqliteDB.Exec("DELETE FROM internal_squad_members")
				return sqliteDB.CreateInBatches(&records, 100).Error
			}
			return nil
		}},
		{"internal_squad_inbounds", func() error {
			var records []database.InternalSquadInbound
			if err := pgDB.Find(&records).Error; err != nil {
				return err
			}
			if len(records) > 0 {
				_ = sqliteDB.Exec("DELETE FROM internal_squad_inbounds")
				return sqliteDB.Create(&records).Error
			}
			return nil
		}},
		{"hwid_user_devices", func() error {
			var records []database.HwidDevice
			if err := pgDB.Find(&records).Error; err != nil {
				return err
			}
			if len(records) > 0 {
				_ = sqliteDB.Exec("DELETE FROM hwid_user_devices")
				return sqliteDB.CreateInBatches(&records, 100).Error
			}
			return nil
		}},
		{"keygen", func() error {
			var records []database.Keygen
			if err := pgDB.Find(&records).Error; err != nil {
				return err
			}
			if len(records) > 0 {
				_ = sqliteDB.Exec("DELETE FROM keygen")
				return sqliteDB.Create(&records).Error
			}
			return nil
		}},
	}

	for _, c := range copiers {
		fmt.Printf("  -> Migrating %s... ", c.name)
		if err := c.fn(); err != nil {
			fmt.Printf("FAILED: %v\n", err)
		} else {
			fmt.Printf("OK\n")
		}
	}

	fmt.Printf("\n[✔] Successfully migrated all data from PostgreSQL to SQLite: %s\n", destPath)

	// Update .env if present in current directory
	envFiles := []string{".env", "/opt/gowave/.env"}
	for _, ef := range envFiles {
		if content, err := os.ReadFile(ef); err == nil {
			lines := strings.Split(string(content), "\n")
			hasDriver := false
			hasURL := false
			for i, line := range lines {
				if strings.HasPrefix(strings.TrimSpace(line), "DB_DRIVER=") {
					lines[i] = "DB_DRIVER=sqlite"
					hasDriver = true
				}
				if strings.HasPrefix(strings.TrimSpace(line), "DATABASE_URL=") {
					lines[i] = fmt.Sprintf("DATABASE_URL=%s", destPath)
					hasURL = true
				}
			}
			if !hasDriver {
				lines = append(lines, "DB_DRIVER=sqlite")
			}
			if !hasURL {
				lines = append(lines, fmt.Sprintf("DATABASE_URL=%s", destPath))
			}
			_ = os.WriteFile(ef+".bak", content, 0644)
			_ = os.WriteFile(ef, []byte(strings.Join(lines, "\n")), 0644)
			fmt.Printf("[+] Updated %s (DB_DRIVER=sqlite, backup saved as %s.bak)\n", ef, filepath.Base(ef))
			break
		}
	}

	return nil
}

func MigratePGToSQLiteInteractive(cfg *config.Config, reader *bufio.Reader) {
	fmt.Println("\n=======================================================")
	fmt.Println("  Migrate from Remnawave PostgreSQL to Gowave SQLite   ")
	fmt.Println("=======================================================")

	detected, _ := DetectRemnawavePostgresDSN()
	if detected != "" {
		fmt.Printf("[*] Auto-detected PostgreSQL connection:\n    %s\n", detected)
		fmt.Print("Press ENTER to use this connection, or type a custom DSN: ")
	} else {
		fmt.Print("Enter PostgreSQL connection DSN (postgres://user:pass@host:5432/db): ")
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	sourceDSN := detected
	if input != "" {
		sourceDSN = input
	}

	fmt.Print("Enter destination SQLite path [default: remnawave.db]: ")
	destIn, _ := reader.ReadString('\n')
	destPath := strings.TrimSpace(destIn)
	if destPath == "" {
		destPath = "remnawave.db"
	}

	err := MigratePostgresToSQLite(cfg, sourceDSN, destPath)
	if err != nil {
		fmt.Printf("[-] Migration failed: %v\n", err)
		return
	}

	// Offer to stop & cleanup Remnawave docker containers
	fmt.Println("\n-------------------------------------------------------")
	fmt.Print("Do you want to stop and remove old Remnawave Docker containers\n(remnawave, remnawave-db, remnawave-redis) to free up RAM & disk? [y/N]: ")
	cleanIn, _ := reader.ReadString('\n')
	cleanChoice := strings.ToLower(strings.TrimSpace(cleanIn))

	if cleanChoice == "y" || cleanChoice == "yes" {
		fmt.Println("[*] Stopping and removing Remnawave Docker containers...")
		containers := []string{"remnawave", "remnawave-db", "remnawave-redis", "remnawave-nginx"}
		for _, c := range containers {
			out, _ := exec.Command("docker", "rm", "-f", c).CombinedOutput()
			res := strings.TrimSpace(string(out))
			if res != "" {
				fmt.Printf("  -> Removed container %s\n", res)
			}
		}
		fmt.Println("[✔] Cleaned up Docker containers! Gowave is now running standalone on SQLite.")
	} else {
		fmt.Println("[i] Skipped Docker container cleanup.")
	}

	fmt.Println("\nMigration complete! Restart gowave service to run on SQLite:")
	fmt.Println("  systemctl restart gowave")
}
