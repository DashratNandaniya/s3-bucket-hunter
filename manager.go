package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

const (
	ResetColor   = "\033[0m"
	BrightRed    = "\033[1;91m"
	BrightGreen  = "\033[1;92m"
	BrightYellow = "\033[1;93m"
	BrightCyan   = "\033[1;96m"
	BrightWhite  = "\033[1;97m"
)

func c(s string, color string) string { return color + s + ResetColor }
func prefixInfo() string             { return c("[*] ", BrightCyan) }
func prefixOk() string               { return c("[✔] ", BrightGreen) }
func prefixWarn() string             { return c("[!] ", BrightYellow) }
func prefixErr() string              { return c("[✘] ", BrightRed) }

// Shared state to keep track of directory context between menus
var currentBrowsingPath = ""

type S3Object struct {
	Key          string
	Size         int64
	LastModified string
}

type S3Index struct {
	Bucket  string
	Objects []S3Object
}

func runCmdPrint(cmd *exec.Cmd) error {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Helper to determine active terminal width dynamically
func getTerminalWidth() int {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err == nil {
		parts := strings.Fields(string(out))
		if len(parts) == 2 {
			if w, err := strconv.Atoi(parts[1]); err == nil && w > 20 {
				return w
			}
		}
	}
	return 90 // Sane high-contrast layout fallback width
}

func buildS3Index(bucket string) (*S3Index, error) {
	fmt.Printf("%s Building recursive index for bucket: %s...\n", prefixInfo(), bucket)
	cmd := exec.Command("aws", "s3", "ls", fmt.Sprintf("s3://%s/", bucket), "--recursive", "--no-sign-request")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to run aws s3 ls: %v", err)
	}

	lines := strings.Split(string(out), "\n")
	objs := make([]S3Object, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" { continue }
		parts := strings.Fields(line)
		if len(parts) < 4 { continue }
		
		size, _ := strconv.ParseInt(parts[2], 10, 64)
		key := strings.Join(parts[3:], " ")
		objs = append(objs, S3Object{Key: key, Size: size, LastModified: parts[0] + " " + parts[1]})
	}

	sort.Slice(objs, func(i, j int) bool { return objs[i].Key < objs[j].Key })
	return &S3Index{Bucket: bucket, Objects: objs}, nil
}

func (idx *S3Index) listChildren(currentPath string) (folders []string, files []S3Object) {
	prefix := strings.Trim(currentPath, "/")
	if prefix != "" { prefix = prefix + "/" }
	folderSet := map[string]bool{}
	fileMap := map[string]S3Object{}

	for _, o := range idx.Objects {
		if !strings.HasPrefix(o.Key, prefix) { continue }
		rem := strings.TrimPrefix(o.Key, prefix)
		if rem == "" { continue }
		parts := strings.SplitN(rem, "/", 2)
		if len(parts) == 1 {
			fileMap[parts[0]] = o
		} else {
			folderSet[parts[0]] = true
		}
	}
	for f := range folderSet { folders = append(folders, f) }
	sort.Strings(folders)
	for _, v := range fileMap { files = append(files, v) }
	sort.Slice(files, func(i, j int) bool { return files[i].Key < files[j].Key })
	return
}

func formatSize(sz int64) string {
	if sz < 1024 { return fmt.Sprintf("%dB", sz) }
	if sz < 1024*1024 { return fmt.Sprintf("%.1fKB", float64(sz)/1024.0) }
	return fmt.Sprintf("%.1fMB", float64(sz)/(1024.0*1024.0))
}

func requiresSudo(dest string) bool {
	dest = strings.TrimSpace(dest)
	if dest == "" { return false }

	testDir := dest
	fi, err := os.Stat(dest)
	if os.IsNotExist(err) {
		idx := strings.LastIndex(dest, "/")
		if idx == 0 {
			testDir = "/"
		} else if idx > 0 {
			testDir = dest[:idx]
		} else {
			testDir = "." 
		}
	} else if err == nil && !fi.IsDir() {
		idx := strings.LastIndex(dest, "/")
		if idx > 0 {
			testDir = dest[:idx]
		} else if idx == 0 {
			testDir = "/"
		} else {
			testDir = "."
		}
	}

	testFile := strings.TrimRight(testDir, "/") + "/.s3_perm_test"
	f, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		if os.IsPermission(err) || strings.Contains(strings.ToLower(err.Error()), "permission denied") {
			return true
		}
		return false
	}
	
	f.Close()
	_ = os.Remove(testFile)
	return false
}

func browseS3(bucket string) {
	reader := bufio.NewReader(os.Stdin)
	idx, err := buildS3Index(bucket)
	if err != nil {
		fmt.Println(prefixErr(), "Failed to index:", err)
		return
	}

	for {
		termWidth := getTerminalWidth()
		s3ContextPath := "s3://" + bucket + "/"
		if currentBrowsingPath != "" {
			s3ContextPath = "s3://" + bucket + "/" + strings.Trim(currentBrowsingPath, "/") + "/"
		}

		fmt.Println("\n" + c(strings.Repeat("=", termWidth), BrightRed))
		fmt.Printf("%s Current Target Location: %s\n", BrightGreen, s3ContextPath)
		fmt.Println(c(strings.Repeat("=", termWidth), BrightRed))

		folders, files := idx.listChildren(currentBrowsingPath)
		type entry struct {
			IsFolder bool
			Name     string
			Object   S3Object
		}
		entries := []entry{}
		for _, f := range folders { entries = append(entries, entry{IsFolder: true, Name: f}) }
		for _, fo := range files {
			parts := strings.Split(fo.Key, "/")
			entries = append(entries, entry{IsFolder: false, Name: parts[len(parts)-1], Object: fo})
		}

		if len(entries) == 0 {
			fmt.Println(prefixWarn() + c("[Empty Location]", BrightYellow))
		} else {
			for i, e := range entries {
				if e.IsFolder {
					fmt.Printf("%2d) %s%s/%s\n", i+1, BrightCyan, e.Name, ResetColor)
				} else {
					fmt.Printf("%2d) %s%s%s (%s)\n", i+1, BrightGreen, e.Name, ResetColor, formatSize(e.Object.Size))
				}
			}
		}

		// Highly organized, single-line horizontal command menu (All characters small)
		colSize := (termWidth - 2) / 8
		if colSize < 12 { colSize = 12 }
		
		fmtFormatStr := fmt.Sprintf("  %%-%ds %%-%ds %%-%ds %%-%ds %%-%ds %%-%ds %%-%ds %%-%ds\n", 
			colSize, colSize, colSize, colSize, colSize, colSize, colSize, colSize)

		fmt.Println("\n" + c(strings.Repeat("-", termWidth), BrightCyan))
		fmt.Printf(fmtFormatStr, 
			c("o <n>:enter", BrightWhite), 
			c("d <n>:download", BrightWhite), 
			c("f:dl-folder", BrightWhite), 
			c("u [path]:upload", BrightWhite), 
			c("r <n>:delete", BrightWhite), 
			c("b:back", BrightWhite), 
			c("l:refresh", BrightWhite), 
			c("q:exit", BrightWhite),
		)
		fmt.Println(c(strings.Repeat("-", termWidth), BrightCyan))
		fmt.Print("\nSelect action command: ")

		cmdLine, _ := reader.ReadString('\n')
		parts := strings.Fields(strings.TrimSpace(cmdLine))
		if len(parts) == 0 { continue }

		// Map to completely lowercase to avoid failure when user uses Shift or Caps Lock
		action := strings.ToLower(parts[0])

		switch action {
		case "l":
			fmt.Println(prefixInfo() + "Rebuilding index...")
			idx, err = buildS3Index(bucket)
			if err != nil {
				fmt.Println(prefixErr(), "Failed to rebuild index:", err)
				return
			}
			fmt.Println(prefixOk() + "Index rebuilt.")
		case "o":
			if len(parts) < 2 {
				fmt.Println(prefixWarn() + "Usage: o <number>")
				continue
			}
			num, _ := strconv.Atoi(parts[1])
			if num < 1 || num > len(entries) {
				fmt.Println(prefixWarn() + "Invalid selection")
				continue
			}
			sel := entries[num-1]
			if sel.IsFolder {
				if currentBrowsingPath == "" { currentBrowsingPath = sel.Name } else { currentBrowsingPath += "/" + sel.Name }
			} else {
				fmt.Println(prefixInfo() + "Displaying file content:")
				_ = runCmdPrint(exec.Command("aws", "s3", "cp", "s3://"+bucket+"/"+sel.Object.Key, "-", "--no-sign-request"))
			}
		case "d":
			if len(parts) < 2 {
				fmt.Println(prefixWarn() + "Usage: d <number>")
				continue
			}
			num, _ := strconv.Atoi(parts[1])
			if num < 1 || num > len(entries) {
				fmt.Println(prefixWarn() + "Invalid selection")
				continue
			}
			sel := entries[num-1]
			fmt.Print(c("Destination local path: ", BrightGreen))
			dest, _ := reader.ReadString('\n')
			dest = strings.TrimSpace(dest)
			if dest == "" { continue }

			var cmdArgs []string
			if requiresSudo(dest) {
				fmt.Println(prefixWarn() + "🔒 Root privileges required. System authentication needed:")
				cmdArgs = append(cmdArgs, "sudo", "aws", "s3", "cp")
			} else {
				cmdArgs = append(cmdArgs, "aws", "s3", "cp")
			}

			if sel.IsFolder {
				srcPath := sel.Name
				if currentBrowsingPath != "" { srcPath = currentBrowsingPath + "/" + sel.Name }
				fmt.Printf("%s Downloading folder s3://%s/%s -> %s\n", prefixInfo(), bucket, srcPath, dest)
				cmdArgs = append(cmdArgs, "s3://"+bucket+"/"+srcPath, dest, "--recursive", "--no-sign-request")
			} else {
				fmt.Printf("%s Downloading file s3://%s/%s -> %s\n", prefixInfo(), bucket, sel.Object.Key, dest)
				cmdArgs = append(cmdArgs, "s3://"+bucket+"/"+sel.Object.Key, dest, "--no-sign-request")
			}

			cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
			_ = runCmdPrint(cmd)

		case "f":
			fmt.Print(c("Destination local path: ", BrightGreen))
			dest, _ := reader.ReadString('\n')
			dest = strings.TrimSpace(dest)
			if dest == "" { continue }

			srcPath := strings.Trim(currentBrowsingPath, "/")
			var cmdArgs []string

			if requiresSudo(dest) {
				fmt.Println(prefixWarn() + "🔒 Root privileges required. System authentication needed:")
				cmdArgs = []string{"sudo", "aws", "s3", "cp", "s3://" + bucket + "/" + srcPath, dest, "--recursive", "--no-sign-request"}
			} else {
				cmdArgs = []string{"aws", "s3", "cp", "s3://" + bucket + "/" + srcPath, dest, "--recursive", "--no-sign-request"}
			}

			fmt.Printf("%s Downloading entire directory s3://%s/%s -> %s\n", prefixInfo(), bucket, srcPath, dest)
			cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
			_ = runCmdPrint(cmd)

		case "u":
			var local string
			// Inline handling: checks if the path was supplied directly (e.g. "u /path/to/file")
			if len(parts) > 1 {
				local = strings.Join(parts[1:], " ")
			} else {
				fmt.Print(c("Local file or folder to upload (path): ", BrightGreen))
				input, _ := reader.ReadString('\n')
				local = strings.TrimSpace(input)
			}

			if local == "" { continue }

			// Safety Boundary Guard: Prevents recursive accidental sync loops of host operating systems
			if local == "/" || local == "/*" {
				fmt.Println(prefixErr() + c("Operation aborted: Uploading the root filesystem context '/' is blocked for safety.", BrightRed))
				continue
			}

			target := "s3://" + bucket + "/" + strings.Trim(currentBrowsingPath, "/")
			var cmd *exec.Cmd
			
			fi, err := os.Stat(local)
			if err != nil {
				fmt.Printf("%s Local path context missing: %v\n", prefixErr(), err)
				continue
			}

			if fi.IsDir() {
				target = strings.TrimRight(target, "/") + "/"
				cmd = exec.Command("aws", "s3", "cp", local, target, "--recursive", "--no-sign-request")
			} else {
				if currentBrowsingPath == "" { target = "s3://" + bucket + "/" } else { target = strings.TrimRight(target, "/") + "/" }
				cmd = exec.Command("aws", "s3", "cp", local, target, "--no-sign-request")
			}
			
			fmt.Printf("%s Uploading %s -> %s\n", prefixInfo(), local, target)
			if err := runCmdPrint(cmd); err != nil {
				fmt.Println("\n" + prefixErr() + c("Upload execution dropped: Target is verified Read-Only.", BrightRed))
			} else {
				fmt.Println(prefixOk() + "Upload complete.")
				idx, _ = buildS3Index(bucket) 
			}
		case "r":
			if len(parts) < 2 {
				fmt.Println(prefixWarn() + "Usage: r <number>")
				continue
			}
			num, _ := strconv.Atoi(parts[1])
			if num < 1 || num > len(entries) {
				fmt.Println(prefixWarn() + "Invalid selection")
				continue
			}
			sel := entries[num-1]
			
			var cmd *exec.Cmd
			if sel.IsFolder {
				srcPath := sel.Name
				if currentBrowsingPath != "" { srcPath = currentBrowsingPath + "/" + sel.Name }
				fmt.Printf("%s Deleting remote folder: s3://%s/%s\n", prefixWarn(), bucket, srcPath)
				cmd = exec.Command("aws", "s3", "rm", "s3://"+bucket+"/"+srcPath, "--recursive", "--no-sign-request")
			} else {
				fmt.Printf("%s Deleting remote file: s3://%s/%s\n", prefixWarn(), bucket, sel.Object.Key)
				cmd = exec.Command("aws", "s3", "rm", "s3://"+bucket+"/"+sel.Object.Key, "--no-sign-request")
			}
			
			if err := runCmdPrint(cmd); err != nil {
				fmt.Println("\n" + prefixErr() + c("Action Denied: This bucket registry instance is Protected/Read-Only.", BrightRed))
			} else {
				fmt.Println(prefixOk() + "Deletion completed.")
				idx, _ = buildS3Index(bucket)
			}
		case "b":
			if currentBrowsingPath != "" {
				pParts := strings.Split(currentBrowsingPath, "/")
				if len(pParts) <= 1 { currentBrowsingPath = "" } else { currentBrowsingPath = strings.Join(pParts[:len(pParts)-1], "/") }
			}
		case "q":
			return
		}
	}
}

func main() {
	reader := bufio.NewReader(os.Stdin)
	for {
		f, err := os.Open("public_buckets.txt")
		if err != nil {
			fmt.Println(prefixErr() + "Could not open 'public_buckets.txt'. Run detector first.")
			return
		}

		var buckets []string
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if b := strings.TrimSpace(scanner.Text()); b != "" { buckets = append(buckets, b) }
		}
		f.Close()

		if len(buckets) == 0 {
			fmt.Println(prefixWarn() + "No public buckets found in 'public_buckets.txt'.")
			return
		}

		termWidth := getTerminalWidth()
		fmt.Println("\n" + c(strings.Repeat("=", termWidth), BrightCyan))
		fmt.Println(c("📂                       DISCOVERED PUBLIC BUCKETS REGISTRY                     📂", BrightCyan))
		fmt.Println(c(strings.Repeat("=", termWidth), BrightCyan))
		
		// Partition screen structure layout into exactly 3 perfectly balanced width columns
		colWidth := (termWidth - 6) / 3
		rowFormatStr := fmt.Sprintf("  %%-%ds | %%-%ds | %%-%ds\n", colWidth, colWidth, colWidth)

		for i := 0; i < len(buckets); i += 3 {
			var col1, col2, col3 string
			
			if i < len(buckets) {
				bName := buckets[i]
				if len(bName) > colWidth-6 { bName = bName[:colWidth-9] + "..." }
				col1 = fmt.Sprintf("%2d) %s", i+1, bName)
			}
			if i+1 < len(buckets) {
				bName := buckets[i+1]
				if len(bName) > colWidth-6 { bName = bName[:colWidth-9] + "..." }
				col2 = fmt.Sprintf("%2d) %s", i+2, bName)
			}
			if i+2 < len(buckets) {
				bName := buckets[i+2]
				if len(bName) > colWidth-6 { bName = bName[:colWidth-9] + "..." }
				col3 = fmt.Sprintf("%2d) %s", i+3, bName)
			}

			fmt.Printf(rowFormatStr, col1, col2, col3)
		}
		
		exitOptionNum := len(buckets) + 1
		fmt.Printf("\n  %2d) %s\n", exitOptionNum, c("exit program", BrightRed))
		fmt.Print("\nSelect a bucket number to open and travel into: ")

		choiceStr, _ := reader.ReadString('\n')
		choice, err := strconv.Atoi(strings.TrimSpace(choiceStr))
		if err != nil || choice < 1 || choice > exitOptionNum {
			fmt.Println(prefixWarn() + "Invalid input matrix choice.")
			continue
		}

		if choice == exitOptionNum {
			fmt.Println(prefixOk() + "Exiting. Goodbye!")
			break
		}

		currentBrowsingPath = "" // Clear context boundaries on travel transition
		browseS3(buckets[choice-1])
	}
}