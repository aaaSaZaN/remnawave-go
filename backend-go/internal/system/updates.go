package system

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
}

func (h *Handler) CheckUpdate(w http.ResponseWriter, r *http.Request) {
	resp, err := http.Get("https://api.github.com/repos/aaaSaZaN/remnawave-go/releases/latest")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"currentVersion": AppVersion,
		"latestVersion":  release.TagName,
		"changelog":      release.Body,
		"hasUpdate":      release.TagName != AppVersion && AppVersion != "unknown",
	})
}

func (h *Handler) ApplyUpdate(w http.ResponseWriter, r *http.Request) {
	exePath, err := os.Executable()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	exePath, _ = filepath.EvalSymlinks(exePath)

	arch := runtime.GOARCH
	binURL := fmt.Sprintf("https://github.com/aaaSaZaN/remnawave-go/releases/latest/download/gowave-linux-%s", arch)
	frontURL := "https://github.com/aaaSaZaN/remnawave-go/releases/latest/download/gowave-frontend.tar.gz"

	tmpBin := exePath + ".new"
	if err := downloadFile(tmpBin, binURL); err != nil {
		http.Error(w, "Failed to download binary: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_ = os.Chmod(tmpBin, 0755)

	distDir := filepath.Join(filepath.Dir(exePath), "frontend", "dist")
	if err := os.MkdirAll(distDir, 0755); err != nil {
		distDir = filepath.Join(filepath.Dir(exePath), "static")
		_ = os.MkdirAll(distDir, 0755)
	}

	if err := downloadAndExtractTarGz(frontURL, distDir); err != nil {
		_ = os.Remove(tmpBin)
		http.Error(w, "Failed to extract frontend: "+err.Error(), http.StatusInternalServerError)
		return
	}

	_ = os.Rename(exePath, exePath+".bak")
	if err := os.Rename(tmpBin, exePath); err != nil {
		_ = os.Rename(exePath+".bak", exePath)
		http.Error(w, "Failed to replace binary: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})

	go func() {
		time.Sleep(2 * time.Second)
		_ = exec.Command("systemctl", "restart", "gowave").Run()
		os.Exit(0)
	}()
}

func downloadFile(filepath string, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func downloadAndExtractTarGz(url, dest string) error {
	cmd := exec.Command("sh", "-c", fmt.Sprintf("curl -sL %s | tar -xz -C %s", url, dest))
	return cmd.Run()
}
