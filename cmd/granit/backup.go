package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type backupFlags struct {
	vaultPath   string
	outputPath  string
	restoreFile string
	listBackups bool
}

func parseBackupFlags(args []string) backupFlags {
	bf := backupFlags{
		vaultPath: ".",
	}

	positional := []string{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--output":
			if i+1 < len(args) {
				bf.outputPath = args[i+1]
				i++
			}
		case "--restore":
			if i+1 < len(args) {
				bf.restoreFile = args[i+1]
				i++
			}
		case "--list":
			bf.listBackups = true
		default:
			positional = append(positional, args[i])
		}
	}

	if len(positional) >= 1 {
		bf.vaultPath = positional[0]
	}

	return bf
}

func runBackup(args []string) {
	bf := parseBackupFlags(args)

	if bf.restoreFile != "" {
		restoreBackup(bf.restoreFile, bf.vaultPath)
		return
	}

	if bf.listBackups {
		listBackups(bf.vaultPath)
		return
	}

	createBackup(bf.vaultPath, bf.outputPath)
}

func createBackup(vaultPath, outputPath string) {
	absPath, err := filepath.Abs(vaultPath)
	if err != nil {
		fmt.Printf("Error resolving path: %v\n", err)
		os.Exit(1)
	}

	// Verify vault directory exists
	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		fmt.Printf("Error: %s is not a valid directory\n", absPath)
		os.Exit(1)
	}

	// Determine backup filename
	vaultName := filepath.Base(absPath)
	timestamp := time.Now().Format("2006-01-02_150405")
	backupName := fmt.Sprintf("%s_backup_%s.zip", vaultName, timestamp)

	// Determine output location
	var backupPath string
	if outputPath != "" {
		absOutput, err := filepath.Abs(outputPath)
		if err != nil {
			fmt.Printf("Error resolving output path: %v\n", err)
			os.Exit(1)
		}
		// If outputPath is a directory, put the file inside it
		if outInfo, err := os.Stat(absOutput); err == nil && outInfo.IsDir() {
			backupPath = filepath.Join(absOutput, backupName)
		} else {
			backupPath = absOutput
		}
	} else {
		// Default: create in .granit/backups/ inside the vault
		backupsDir := filepath.Join(absPath, ".granit", "backups")
		if err := os.MkdirAll(backupsDir, 0755); err != nil {
			fmt.Printf("Error creating backups directory: %v\n", err)
			os.Exit(1)
		}
		backupPath = filepath.Join(backupsDir, backupName)
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(backupPath), 0755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Creating backup of %s\n", absPath)

	// Create zip file
	zipFile, err := os.Create(backupPath)
	if err != nil {
		fmt.Printf("Error creating backup file: %v\n", err)
		os.Exit(1)
	}

	w := zip.NewWriter(zipFile)

	fileCount := 0
	var totalSize int64

	err = filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the backups directory itself to avoid recursion
		if info.IsDir() && path == filepath.Join(absPath, ".granit", "backups") {
			return filepath.SkipDir
		}

		// Skip .git directory
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}

		relPath, _ := filepath.Rel(absPath, path)

		if info.IsDir() {
			// Add directory entry
			_, err := w.Create(relPath + "/")
			return err
		}

		// Create file header with modification time
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = relPath
		header.Method = zip.Deflate

		writer, err := w.CreateHeader(header)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()

		written, err := io.Copy(writer, file)
		if err != nil {
			return err
		}

		fileCount++
		totalSize += written
		return nil
	})

	if err != nil {
		// Close and clean up on error
		_ = w.Close()
		_ = zipFile.Close()
		fmt.Printf("Error creating backup: %v\n", err)
		os.Exit(1)
	}

	// Close the zip writer and file before reading size
	if err := w.Close(); err != nil {
		_ = zipFile.Close()
		fmt.Printf("Error finalizing backup: %v\n", err)
		os.Exit(1)
	}
	_ = zipFile.Close()

	// Get final backup file size
	backupInfo, _ := os.Stat(backupPath)
	backupSize := backupInfo.Size()

	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("Backup created: %s\n", backupPath)
	fmt.Printf("Files: %d\n", fileCount)
	fmt.Printf("Original size: %s\n", formatSize(totalSize))
	fmt.Printf("Backup size: %s\n", formatSize(backupSize))
}

func listBackups(vaultPath string) {
	absPath, err := filepath.Abs(vaultPath)
	if err != nil {
		fmt.Printf("Error resolving path: %v\n", err)
		os.Exit(1)
	}

	backupsDir := filepath.Join(absPath, ".granit", "backups")
	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No backups found.")
			fmt.Printf("Backups directory: %s\n", backupsDir)
			return
		}
		fmt.Printf("Error reading backups directory: %v\n", err)
		os.Exit(1)
	}

	var backups []backupEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		backups = append(backups, backupEntry{
			name:    entry.Name(),
			size:    info.Size(),
			modTime: info.ModTime(),
			path:    filepath.Join(backupsDir, entry.Name()),
		})
	}

	if len(backups) == 0 {
		fmt.Println("No backups found.")
		fmt.Printf("Backups directory: %s\n", backupsDir)
		return
	}

	// Sort by modification time, newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].modTime.After(backups[j].modTime)
	})

	fmt.Println("Available backups:")
	fmt.Println(strings.Repeat("-", 70))
	fmt.Printf("  %-40s  %-10s  %s\n", "FILENAME", "SIZE", "DATE")
	fmt.Println(strings.Repeat("-", 70))

	for _, b := range backups {
		fmt.Printf("  %-40s  %-10s  %s\n",
			b.name,
			formatSize(b.size),
			b.modTime.Format("2006-01-02 15:04:05"),
		)
	}
	fmt.Println(strings.Repeat("-", 70))
	fmt.Printf("  %d backup(s) found\n", len(backups))
	fmt.Printf("  Restore with: granit backup --restore <backup-file> [vault-path]\n")
}

type backupEntry struct {
	name    string
	size    int64
	modTime time.Time
	path    string
}

func restoreBackup(backupFile, vaultPath string) {
	absBackup, err := filepath.Abs(backupFile)
	if err != nil {
		fmt.Printf("Error resolving backup path: %v\n", err)
		os.Exit(1)
	}

	// Verify backup file exists
	if _, err := os.Stat(absBackup); os.IsNotExist(err) {
		fmt.Printf("Error: backup file not found: %s\n", absBackup)
		os.Exit(1)
	}

	absDest, err := filepath.Abs(vaultPath)
	if err != nil {
		fmt.Printf("Error resolving destination path: %v\n", err)
		os.Exit(1)
	}

	// Create destination if it doesn't exist
	if err := os.MkdirAll(absDest, 0755); err != nil {
		fmt.Printf("Error creating destination directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Restoring backup: %s\n", absBackup)
	fmt.Printf("Destination: %s\n", absDest)
	fmt.Println(strings.Repeat("-", 50))

	// Open zip file
	reader, err := zip.OpenReader(absBackup)
	if err != nil {
		fmt.Printf("Error opening backup: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = reader.Close() }()

	fileCount := 0

	for _, file := range reader.File {
		destPath := filepath.Join(absDest, file.Name)

		// Security: prevent zip slip
		if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(absDest)+string(os.PathSeparator)) &&
			filepath.Clean(destPath) != filepath.Clean(absDest) {
			fmt.Printf("  Skipping unsafe path: %s\n", file.Name)
			continue
		}

		if file.FileInfo().IsDir() {
			_ = os.MkdirAll(destPath, file.Mode())
			continue
		}

		// Create parent directory
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			fmt.Printf("  Error creating directory for %s: %v\n", file.Name, err)
			continue
		}

		// Extract file
		srcFile, err := file.Open()
		if err != nil {
			fmt.Printf("  Error reading %s: %v\n", file.Name, err)
			continue
		}

		dstFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode())
		if err != nil {
			_ = srcFile.Close()
			fmt.Printf("  Error writing %s: %v\n", file.Name, err)
			continue
		}

		_, err = io.Copy(dstFile, srcFile)
		_ = srcFile.Close()
		_ = dstFile.Close()

		if err != nil {
			fmt.Printf("  Error extracting %s: %v\n", file.Name, err)
			continue
		}

		fileCount++
	}

	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("Restore complete: %d files restored to %s\n", fileCount, absDest)
}

// formatSize returns a human-readable file size string.
func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
