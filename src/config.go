package src

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	AppName string      `json:"app_name"`
	Hosts   []Host      `json:"hosts"`
	Tools   ToolConfigs `json:"tools"`
}

type Host struct {
	HostID          string `json:"host_id"`
	HostName        string `json:"host_name"`
	HostAddr        string `json:"host_addr"`
	HostPort        string `json:"host_port"`
	HostSSHUsername string `json:"host_ssh_username"`
	HostSSHPassword string `json:"host_ssh_password"`
}

type ToolConfigs struct {
	Download          []TransferTask `json:"download"`
	Upload            []UploadTask   `json:"upload"`
	ImportRDBLocalDir string         `json:"import_rdb_local_dir"`
}

type TransferTask struct {
	IsDir     bool   `json:"is_dir"`
	SrcDir    string `json:"src_dir"`
	TargetDir string `json:"target_dir"`
}

type UploadTask struct {
	SrcDir    string   `json:"src_dir"`
	TargetDir string   `json:"target_dir"`
	IsDir     bool     `json:"is_dir"`
	ThenRun   []string `json:"then_run"`
}

func LoadConfig() (Config, string, error) {
	paths := []string{
		"config.json",
		filepath.Join(executableDir(), "config.json"),
	}

	var lastErr error
	for _, candidate := range paths {
		configPath, err := filepath.Abs(candidate)
		if err != nil {
			lastErr = err
			continue
		}

		content, err := os.ReadFile(configPath)
		if err != nil {
			lastErr = err
			continue
		}

		var cfg Config
		if err := json.Unmarshal(content, &cfg); err != nil {
			return Config{}, "", fmt.Errorf("解析配置失败 %s: %w", configPath, err)
		}
		return cfg, configPath, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("未找到配置文件")
	}
	return Config{}, "", lastErr
}

func executableDir() string {
	exePath, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exePath)
}
