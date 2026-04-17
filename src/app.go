package src

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type App struct {
	Config     Config
	ConfigPath string
	Reader     *bufio.Reader
}

func NewApp(cfg Config, configPath string) *App {
	return &App{
		Config:     cfg,
		ConfigPath: configPath,
		Reader:     bufio.NewReader(os.Stdin),
	}
}

func (a *App) Run() error {
	if len(a.Config.Hosts) == 0 {
		return errors.New("配置中没有 hosts")
	}

	fmt.Printf("=== %s ===\n", a.Config.AppName)
	fmt.Printf("配置文件: %s\n\n", a.ConfigPath)

	hostIndex, err := a.chooseHost()
	if err != nil {
		return err
	}

	for {
		host := a.Config.Hosts[hostIndex]
		choice, err := a.promptChoice(
			fmt.Sprintf("当前主机: %s (%s@%s:%s)", host.HostName, host.HostSSHUsername, host.HostAddr, host.HostPort),
			[]string{
				"执行自定义命令",
				"下载预设文件/目录",
				"上传预设文件/目录",
				"导入本地 RDB 到远端文件",
				"切换主机",
				"退出程序",
			},
		)
		if err != nil {
			return err
		}

		switch choice {
		case 0:
			if err := a.runCustomCommand(host); err != nil {
				fmt.Printf("执行失败: %v\n\n", err)
			}
		case 1:
			if err := a.runDownload(host); err != nil {
				fmt.Printf("下载失败: %v\n\n", err)
			}
		case 2:
			if err := a.runUpload(host); err != nil {
				fmt.Printf("上传失败: %v\n\n", err)
			}
		case 3:
			if err := a.runRDBImport(host); err != nil {
				fmt.Printf("导入失败: %v\n\n", err)
			}
		case 4:
			hostIndex, err = a.chooseHost()
			if err != nil {
				return err
			}
		case 5:
			fmt.Println("已退出。")
			return nil
		}
	}
}

func (a *App) chooseHost() (int, error) {
	options := make([]string, 0, len(a.Config.Hosts))
	for _, host := range a.Config.Hosts {
		options = append(options, fmt.Sprintf("%s (%s@%s:%s)", host.HostName, host.HostSSHUsername, host.HostAddr, host.HostPort))
	}

	return a.promptChoice("请选择要操作的主机", options)
}

func (a *App) runCustomCommand(host Host) error {
	command, err := a.promptText("请输入要执行的命令")
	if err != nil {
		return err
	}
	if command == "" {
		fmt.Println("未输入命令，已取消。\n")
		return nil
	}

	fmt.Printf("\n连接 %s，执行命令中...\n", host.HostName)
	client, err := NewSSHClient(host)
	if err != nil {
		return err
	}
	defer client.Close()

	output, err := client.RunCommand(command)
	fmt.Println("----- 命令回显 -----")
	if strings.TrimSpace(output) == "" {
		fmt.Println("(无输出)")
	} else {
		fmt.Println(output)
	}
	fmt.Println("--------------------\n")
	return err
}

func (a *App) runDownload(host Host) error {
	if len(a.Config.Tools.Download) == 0 {
		fmt.Println("没有配置 download 任务。\n")
		return nil
	}

	options := make([]string, 0, len(a.Config.Tools.Download))
	for _, task := range a.Config.Tools.Download {
		taskType := "文件"
		if task.IsDir {
			taskType = "目录"
		}
		options = append(options, fmt.Sprintf("[%s] %s -> %s", taskType, task.SrcDir, task.TargetDir))
	}

	taskIndex, err := a.promptChoice("请选择下载任务", append(options, "返回上一级"))
	if err != nil {
		return err
	}
	if taskIndex == len(options) {
		fmt.Println()
		return nil
	}

	task := a.Config.Tools.Download[taskIndex]
	client, err := NewSSHClient(host)
	if err != nil {
		return err
	}
	defer client.Close()

	fmt.Printf("\n正在下载 %s -> %s\n", task.SrcDir, task.TargetDir)
	if task.IsDir {
		err = client.DownloadDir(task.SrcDir, task.TargetDir)
	} else {
		err = client.DownloadFile(task.SrcDir, task.TargetDir)
	}
	if err != nil {
		return err
	}

	fmt.Println("下载完成。\n")
	return nil
}

func (a *App) runUpload(host Host) error {
	if len(a.Config.Tools.Upload) == 0 {
		fmt.Println("没有配置 upload 任务。\n")
		return nil
	}

	options := make([]string, 0, len(a.Config.Tools.Upload))
	for _, task := range a.Config.Tools.Upload {
		taskType := "文件"
		if task.IsDir {
			taskType = "目录"
		}
		options = append(options, fmt.Sprintf("[%s] %s -> %s", taskType, task.SrcDir, task.TargetDir))
	}

	taskIndex, err := a.promptChoice("请选择上传任务", append(options, "返回上一级"))
	if err != nil {
		return err
	}
	if taskIndex == len(options) {
		fmt.Println()
		return nil
	}

	task := a.Config.Tools.Upload[taskIndex]
	client, err := NewSSHClient(host)
	if err != nil {
		return err
	}
	defer client.Close()

	fmt.Printf("\n正在上传 %s -> %s\n", task.SrcDir, task.TargetDir)
	if task.IsDir {
		err = client.UploadDir(task.SrcDir, task.TargetDir)
	} else {
		err = client.UploadFile(task.SrcDir, task.TargetDir)
	}
	if err != nil {
		return err
	}
	fmt.Println("上传完成。")

	if len(task.ThenRun) > 0 {
		fmt.Println("\n开始执行上传后的命令:")
		for _, command := range task.ThenRun {
			fmt.Printf("\n$ %s\n", command)
			output, runErr := client.RunCommand(command)
			if strings.TrimSpace(output) == "" {
				fmt.Println("(无输出)")
			} else {
				fmt.Println(output)
			}
			if runErr != nil {
				return fmt.Errorf("命令执行失败 %q: %w", command, runErr)
			}
		}
	}

	fmt.Println()
	return nil
}

func (a *App) runRDBImport(host Host) error {
	localPath := a.Config.Tools.ImportRDBLocalDir
	if localPath == "" {
		return errors.New("config.json 未配置 tools.import_rdb_local_dir")
	}

	localAbs, err := filepath.Abs(localPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(localAbs)
	if err != nil {
		return fmt.Errorf("读取本地 RDB 失败: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("RDB 路径不是文件: %s", localAbs)
	}

	fmt.Printf("\n本地 RDB: %s\n", localAbs)
	remotePath, err := a.promptTextWithDefault("请输入远端 dump.rdb 路径", "/var/lib/redis/dump.rdb")
	if err != nil {
		return err
	}
	backupChoice, err := a.promptTextWithDefault("是否备份远端原文件? (y/n)", "y")
	if err != nil {
		return err
	}
	restartCommand, err := a.promptTextWithDefault("上传后执行什么重启命令? (留空则跳过)", "")
	if err != nil {
		return err
	}

	client, err := NewSSHClient(host)
	if err != nil {
		return err
	}
	defer client.Close()

	if strings.EqualFold(backupChoice, "y") || strings.EqualFold(backupChoice, "yes") {
		backupPath := fmt.Sprintf("%s.bak.%s", remotePath, time.Now().Format("20060102_150405"))
		command := fmt.Sprintf("if [ -f %q ]; then cp %q %q; fi", remotePath, remotePath, backupPath)
		output, runErr := client.RunCommand(command)
		if strings.TrimSpace(output) != "" {
			fmt.Printf("\n备份回显:\n%s\n", output)
		}
		if runErr != nil {
			return fmt.Errorf("备份远端 RDB 失败: %w", runErr)
		}
		fmt.Printf("远端备份路径: %s\n", backupPath)
	}

	fmt.Printf("开始上传 %s -> %s\n", localAbs, remotePath)
	if err := client.UploadFile(localAbs, remotePath); err != nil {
		return err
	}
	fmt.Println("RDB 上传完成。")

	if restartCommand != "" {
		fmt.Printf("\n执行重启命令: %s\n", restartCommand)
		output, runErr := client.RunCommand(restartCommand)
		if strings.TrimSpace(output) == "" {
			fmt.Println("(无输出)")
		} else {
			fmt.Println(output)
		}
		if runErr != nil {
			return fmt.Errorf("重启命令执行失败: %w", runErr)
		}
	}

	fmt.Println()
	return nil
}

func (a *App) promptChoice(title string, options []string) (int, error) {
	if len(options) == 0 {
		return 0, errors.New("没有可选项")
	}

	for {
		fmt.Println(title)
		for i, option := range options {
			fmt.Printf("  %d. %s\n", i+1, option)
		}
		fmt.Print("请输入序号: ")

		raw, err := a.readLine()
		if err != nil {
			return 0, err
		}

		index, err := strconv.Atoi(raw)
		if err != nil || index < 1 || index > len(options) {
			fmt.Println("输入无效，请重新输入。\n")
			continue
		}

		fmt.Println()
		return index - 1, nil
	}
}

func (a *App) promptText(label string) (string, error) {
	fmt.Printf("%s: ", label)
	return a.readLine()
}

func (a *App) promptTextWithDefault(label, defaultValue string) (string, error) {
	if defaultValue == "" {
		fmt.Printf("%s: ", label)
	} else {
		fmt.Printf("%s [%s]: ", label, defaultValue)
	}

	value, err := a.readLine()
	if err != nil {
		return "", err
	}
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}

func (a *App) readLine() (string, error) {
	line, err := a.Reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
