package system

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"

	"gorm.io/gorm"
)

type BackupRestoreService struct {
	db *gorm.DB
}

func NewBackupRestoreService(db *gorm.DB) *BackupRestoreService {
	return &BackupRestoreService{db: db}
}

func (s *BackupRestoreService) RestoreFromTarGz(tarGzPath string) error {
	file, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if strings.HasSuffix(header.Name, ".sql.gz") {
			return s.restoreSqlGz(tr)
		}
	}

	return fmt.Errorf("dump.sql.gz not found in archive")
}

func (s *BackupRestoreService) restoreSqlGz(r io.Reader) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gzr.Close()

	scanner := bufio.NewScanner(gzr)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var currentTable string
	var columns []string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "COPY public.") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				tableWithPublic := parts[1]
				currentTable = strings.TrimPrefix(tableWithPublic, "public.")
				colPart := line[strings.Index(line, "(")+1 : strings.Index(line, ")")]
				columns = strings.Split(colPart, ", ")
			}
			continue
		}

		if currentTable != "" {
			if line == "\\." {
				currentTable = ""
				columns = nil
				continue
			}

			values := strings.Split(line, "\t")
			if len(values) == len(columns) {
				s.insertRow(currentTable, columns, values)
			}
		}
	}

	return scanner.Err()
}

func (s *BackupRestoreService) insertRow(table string, columns []string, values []string) {
	var cleanCols []string
	var placeholders []string
	var args []interface{}

	for i, col := range columns {
		cleanCols = append(cleanCols, fmt.Sprintf(`"%s"`, col))
		placeholders = append(placeholders, "?")

		val := values[i]
		if val == "\\N" {
			args = append(args, nil)
		} else {
			args = append(args, val)
		}
	}

	query := fmt.Sprintf(`INSERT INTO "%s" (%s) VALUES (%s)`,
		table, strings.Join(cleanCols, ", "), strings.Join(placeholders, ", "))

	s.db.Exec(query, args...)
}
