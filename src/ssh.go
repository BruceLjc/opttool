package src

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type SSHClient struct {
	host       Host
	sshClient  *ssh.Client
	sftpClient *sftp.Client
}

func NewSSHClient(host Host) (*SSHClient, error) {
	config := &ssh.ClientConfig{
		User: host.HostSSHUsername,
		Auth: []ssh.AuthMethod{
			ssh.Password(host.HostSSHPassword),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	address := fmt.Sprintf("%s:%s", host.HostAddr, host.HostPort)
	sshClient, err := ssh.Dial("tcp", address, config)
	if err != nil {
		return nil, fmt.Errorf("连接 SSH 失败 %s: %w", address, err)
	}

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		sshClient.Close()
		return nil, fmt.Errorf("创建 SFTP 客户端失败: %w", err)
	}

	return &SSHClient{
		host:       host,
		sshClient:  sshClient,
		sftpClient: sftpClient,
	}, nil
}

func (c *SSHClient) Close() {
	if c.sftpClient != nil {
		c.sftpClient.Close()
	}
	if c.sshClient != nil {
		c.sshClient.Close()
	}
}

func (c *SSHClient) RunCommand(command string) (string, error) {
	session, err := c.sshClient.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	return strings.TrimRight(string(output), "\n"), err
}

func (c *SSHClient) UploadFile(localPath, remotePath string) error {
	localAbs, err := filepath.Abs(localPath)
	if err != nil {
		return err
	}

	source, err := os.Open(localAbs)
	if err != nil {
		return fmt.Errorf("打开本地文件失败 %s: %w", localAbs, err)
	}
	defer source.Close()

	if err := c.sftpClient.MkdirAll(path.Dir(remotePath)); err != nil {
		return fmt.Errorf("创建远端目录失败 %s: %w", path.Dir(remotePath), err)
	}

	target, err := c.sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("创建远端文件失败 %s: %w", remotePath, err)
	}
	defer target.Close()

	if _, err := io.Copy(target, source); err != nil {
		return fmt.Errorf("上传文件失败 %s: %w", remotePath, err)
	}
	return nil
}

func (c *SSHClient) DownloadFile(remotePath, localPath string) error {
	source, err := c.sftpClient.Open(remotePath)
	if err != nil {
		return fmt.Errorf("打开远端文件失败 %s: %w", remotePath, err)
	}
	defer source.Close()

	localAbs, err := filepath.Abs(localPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(localAbs), 0o755); err != nil {
		return fmt.Errorf("创建本地目录失败 %s: %w", filepath.Dir(localAbs), err)
	}

	target, err := os.Create(localAbs)
	if err != nil {
		return fmt.Errorf("创建本地文件失败 %s: %w", localAbs, err)
	}
	defer target.Close()

	if _, err := io.Copy(target, source); err != nil {
		return fmt.Errorf("下载文件失败 %s: %w", remotePath, err)
	}
	return nil
}

func (c *SSHClient) UploadDir(localDir, remoteDir string) error {
	localAbs, err := filepath.Abs(localDir)
	if err != nil {
		return err
	}

	return filepath.Walk(localAbs, func(current string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relative, err := filepath.Rel(localAbs, current)
		if err != nil {
			return err
		}
		remotePath := path.Join(remoteDir, filepath.ToSlash(relative))

		if info.IsDir() {
			return c.sftpClient.MkdirAll(remotePath)
		}
		return c.UploadFile(current, remotePath)
	})
}

func (c *SSHClient) DownloadDir(remoteDir, localDir string) error {
	remoteInfo, err := c.sftpClient.Stat(remoteDir)
	if err != nil {
		return fmt.Errorf("读取远端目录失败 %s: %w", remoteDir, err)
	}
	if !remoteInfo.IsDir() {
		return fmt.Errorf("远端路径不是目录: %s", remoteDir)
	}

	localAbs, err := filepath.Abs(localDir)
	if err != nil {
		return err
	}
	return c.downloadDirRecursive(remoteDir, localAbs)
}

func (c *SSHClient) downloadDirRecursive(remoteDir, localDir string) error {
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return err
	}

	entries, err := c.sftpClient.ReadDir(remoteDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		remotePath := path.Join(remoteDir, entry.Name())
		localPath := filepath.Join(localDir, entry.Name())

		if entry.IsDir() {
			if err := c.downloadDirRecursive(remotePath, localPath); err != nil {
				return err
			}
			continue
		}

		if err := c.DownloadFile(remotePath, localPath); err != nil {
			return err
		}
	}
	return nil
}
