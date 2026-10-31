// Command setup downloads and unpacks the IBM clidriver for the
// current platform; run it via `go run installer/setup.go`.
package main

import (
	"archive/zip"
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	primaryURL  = "https://public.dhe.ibm.com/ibmdl/export/pub/software/data/db2/drivers/odbc_cli/"
	fallbackURL = "https://github.com/ibmdb/db2drivers/raw/main/clidriver/"
)

func fileExists(path string) bool {
	_, err := os.Stat(path) //nolint:gosec // G703: helper with caller-controlled paths, used by the installer only
	return err == nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path) //nolint:gosec // G703: helper with caller-controlled paths, used by the installer only
	return err == nil && info.IsDir()
}

func downloadFile(url string, filename string) error {
	// Create HTTP GET request
	resp, err := http.Get(url) //nolint:gosec // G107: the installer is designed to accept a download URL from the environment
	if err != nil {
		return fmt.Errorf("failed to fetch %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check for non-200 status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download %s: HTTP %d", url, resp.StatusCode)
	}

	// Create destination file
	out, err := os.Create("../../" + filename) //nolint:gosec // G304: installer-only tool, filename is derived from the fixed IBM download
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", filename, err)
	}
	defer func() { _ = out.Close() }()

	// Copy data
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to save file %s: %w", filename, err)
	}

	return nil
}

func Unzipping(sourcefile string, targetDirectory string) {
	if err := unzip(sourcefile, targetDirectory); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// maxDecompressedFileSize caps per-file extracted output so a corrupt or
// hostile archive cannot exhaust the disk (decompression bomb). The
// clidriver payloads are far below this limit.
const maxDecompressedFileSize = 1 << 31 // 2 GiB

func unzip(sourcefile string, targetDirectory string) error {
	reader, err := zip.OpenReader("./../../" + sourcefile) //nolint:gosec // installer-only tool: path is fixed relative to the repo
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	for _, f := range reader.File {
		if err := extractZipEntry(f, targetDirectory); err != nil {
			return err
		}
	}
	return nil
}

func extractZipEntry(f *zip.File, targetDirectory string) error {
	zipped, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = zipped.Close() }()

	path := filepath.Join(targetDirectory, f.Name) //nolint:gosec // G305: checked by the zip-slip guard below
	// zip-slip guard: reject entries that escape the target directory
	cleanTarget := filepath.Clean(targetDirectory)
	if path != cleanTarget && !strings.HasPrefix(path, cleanTarget+string(os.PathSeparator)) {
		return fmt.Errorf("archive entry %q escapes target directory", f.Name)
	}
	if f.FileInfo().IsDir() {
		if err := os.MkdirAll(path, f.Mode()); err != nil { //nolint:gosec // G703: path is checked by the zip-slip guard above
			return fmt.Errorf("creating directory %s: %w", path, err)
		}
		fmt.Println("Creating directory", path)
		return nil
	}
	writer, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, f.Mode()) //nolint:gosec // G304: path is checked by the zip-slip guard above
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(writer, io.LimitReader(zipped, maxDecompressedFileSize+1))
	if cerr := writer.Close(); copyErr == nil {
		copyErr = cerr
	}
	if copyErr != nil {
		return copyErr
	}
	if n > maxDecompressedFileSize {
		return fmt.Errorf("archive entry %q decompresses beyond %d bytes", f.Name, maxDecompressedFileSize)
	}
	fmt.Println("Unzipping : ", path)
	return nil
}

func linuxUntar(clidriver string, targetDirectory string, ibmdbDir string) error {
	fmt.Printf("Extracting with tar -xzf %s -C %s", clidriver, targetDirectory)
	out, err := exec.CommandContext(context.Background(), "tar", "xzf", "./../../"+clidriver, "-C", targetDirectory).Output() //nolint:gosec // G204: installer tool, arguments are built from the fixed IBM archive name

	fmt.Println(string(out))
	if err != nil {
		fmt.Println("Error while running tar: " + err.Error())
		return err
	}
	fmt.Println("Extracted clidriver path = " + targetDirectory + "/clidriver")
	fmt.Println("\nNow run below commands:")
	fmt.Println("export IBM_DB_HOME=" + targetDirectory + "/clidriver")
	fmt.Println("source " + ibmdbDir + "/installer/setenv.sh")

	if runtime.GOOS == "darwin" {
		// Create symlinks for libdb2
		_, _ = exec.Command("ln", "-s", targetDirectory+"/clidriver/lib/libdb2.dylib", targetDirectory+"/libdb2.dylib").Output()
		_, _ = exec.Command("ln", "-s", targetDirectory+"/clidriver/lib/libdb2.dylib", ibmdbDir+"/libdb2.dylib").Output()
		_, _ = exec.Command("ln", "-s", targetDirectory+"/clidriver/lib/libdb2.dylib", ibmdbDir+"/testdata/libdb2.dylib").Output()
	}

	return nil
}

func aixUntar(clidriver string, targetDirectory string) error {
	fmt.Printf("Extracting with gunzip %s", clidriver)
	gunzipOut, err := exec.Command("gunzip", "./../../"+clidriver).Output() //nolint:gosec // G204: installer tool, argument is the fixed IBM archive name

	fmt.Println(string(gunzipOut))
	if err != nil {
		fmt.Println("Error while running gunzip: " + err.Error())
		return err
	}

	clidriver = strings.TrimRight(clidriver, ".gz")

	fmt.Printf("Extracting with tar -xvf %s -C %s", clidriver, targetDirectory)
	tarOut, err := exec.Command("tar", "xvf", "./../../"+clidriver, "-C", targetDirectory).Output() //nolint:gosec // G204/G305: installer tool, arguments are the fixed IBM archive and target

	fmt.Println(string(tarOut))
	if err != nil {
		fmt.Println("Error while running tar: " + err.Error())
		return err
	}
	fmt.Println("Extracted clidriver path = " + targetDirectory + "/clidriver")
	fmt.Println("\nNow run below commands:")
	fmt.Println("export IBM_DB_HOME=" + targetDirectory + "/clidriver")
	fmt.Println("export CGO_CFLAGS==\"-I$IBM_DB_HOME/include\"")
	fmt.Println("export CGO_LDFLAGS==\"-L$IBM_DB_HOME/lib\"")

	return nil
}

func checkInstalledDriver(validateout string) bool {
	var line string

	scanner := bufio.NewScanner(strings.NewReader(validateout))

	for scanner.Scan() {
		line = scanner.Text()

		if strings.Contains(line, "Install") {
			fields := strings.Split(line, " ")
			// fmt.Println(fields[7])
			input1 := fields[7][0:len(fields[7])]
			fmt.Println("Found installed Db2 Client as", input1)
			if checkIncludeDir(input1) {
				fmt.Println("Please set IBM_DB_HOME to", input1)
				return true
			}
		}
	}
	return false
}

func checkIncludeDir(clidriver string) bool {
	if dirExists(clidriver + "/include") {
		// fmt.Println("clidriver/include folder exists in the path", clidriver)
		if !dirExists(clidriver + "/lib") {
			fmt.Println(clidriver + "/lib directory does not exist.")
			return false
		}
		// fmt.Println("clidriver/lib folder exists in the path", clidriver)
		if !fileExists(clidriver + "/include/sqlcli.h") {
			fmt.Println(clidriver + "/include/sqlcli.h file does not exist.")
			return false
		}
		return true
	}
	fmt.Println(clidriver + "/include directory does not exist.")
	return false
}

func main() {
	var target, ibmdbDir, cliFileName, existingDriver string
	var unpackageType int
	var err11 error
	var out []byte

	fmt.Println("os =", runtime.GOOS, ", processor =", runtime.GOARCH)
	path, ok := os.LookupEnv("DB2HOME")
	if ok {
		fmt.Println("NOTE: Environment variable DB2HOME name is changed to IBM_DB_HOME.")
		fmt.Println("DB2HOME environment variable is set to", path)
	}

	out, err11 = exec.CommandContext(context.Background(), "db2cli", "validate").Output()
	if err11 != nil {
		// db2cli validate is not working, check for IBM_DB_HOME
		ibmDBHome, found := os.LookupEnv("IBM_DB_HOME")
		if !found || ibmDBHome == "" || !dirExists(ibmDBHome) {
			// IBM_DB_HOME is not set
			switch runtime.GOOS {
			case "windows":
				fmt.Println("Please set IBM_DB_HOME and add %IBM_DB_HOME%/bin to PATH and %IBM_DB_HOME%/lib to LIB environment variables after clidriver installation")
			case "aix":
				fmt.Println("Please set IBM_DB_HOME, CGO_CFLAGS, CGO_LDFLAGS and LIBPATH environment variables after clidriver installation")
			case "darwin":
				fmt.Println("Please set IBM_DB_HOME, CGO_CFLAGS, CGO_LDFLAGS and DYLD_LIBRARY_PATH environment variables after clidriver installation")
			default:
				fmt.Println("Please set IBM_DB_HOME, CGO_CFLAGS, CGO_LDFLAGS and LD_LIBRARY_PATH environment variables after clidriver installation")
			}
		} else {
			fmt.Println("IBM_DB_HOME is set to", ibmDBHome)
		}
	} else {
		fmt.Println("db2cli validate command is working.")
		ibmDBHome, found := os.LookupEnv("IBM_DB_HOME")
		if !found || ibmDBHome == "" || !dirExists(ibmDBHome) {
			// Check install path in validate output
			if checkInstalledDriver(string(out)) {
				os.Exit(1)
			}
		} else {
			fmt.Println("IBM_DB_HOME =", ibmDBHome)
			if checkIncludeDir(ibmDBHome) {
				fmt.Println("IBM_DB_HOME is set and directory exists. Skipping download.")
				os.Exit(1)
			}
		}
	}

	_, setupFile, _, ok := runtime.Caller(0)
	if ok {
		ibmdbDir = filepath.Dir(setupFile) + "/.."
	} else {
		ibmdbDir = "./.."
	}
	target = ibmdbDir + "/.."
	ibmdbDir, _ = filepath.Abs(ibmdbDir)
	target, _ = filepath.Abs(target)

	if len(os.Args) == 2 {
		target = os.Args[1]
	}
	existingDriver, _ = filepath.Abs(target + "/clidriver")

	if dirExists(existingDriver) {
		fmt.Println("Detected existing directory \"" + existingDriver + "\"")

		if checkIncludeDir(existingDriver) {
			fmt.Println("==> Skipping clidriver download.")
			os.Exit(2)
		}
		// Attempt to remove the directory and its contents
		err2 := os.RemoveAll(existingDriver) //nolint:gosec // G703: installer removes its own previously extracted directory
		if err2 != nil {
			fmt.Printf("Error deleting directory '%s': %v\n", existingDriver, err2)
		} else {
			fmt.Printf("Directory '%s' and its contents successfully deleted.\n", existingDriver)
		}
		fmt.Println("==> Installing clidriver ...")
	}

	switch runtime.GOOS {
	case "windows":
		unpackageType = 1
		const wordsize = 32 << (^uint(0) >> 32 & 1)
		if wordsize == 64 {
			cliFileName = "ntx64_odbc_cli.zip"
		} else {
			cliFileName = "nt32_odbc_cli.zip"
		}
	case "linux":
		unpackageType = 2
		switch runtime.GOARCH {
		case "ppc64le":
			const wordsize = 32 << (^uint(0) >> 32 & 1)
			if wordsize == 64 {
				cliFileName = "ppc64le_odbc_cli.tar.gz"
			}
		case "ppc":
			const wordsize = 32 << (^uint(0) >> 32 & 1)
			if wordsize == 64 {
				cliFileName = "ppc64_odbc_cli.tar.gz"
			} else {
				cliFileName = "ppc32_odbc_cli.tar.gz"
			}
		case "amd64":
			const wordsize = 32 << (^uint(0) >> 32 & 1)
			if wordsize == 64 {
				cliFileName = "linuxx64_odbc_cli.tar.gz"
			} else {
				cliFileName = "linuxia32_odbc_cli.tar.gz"
			}
		case "s390x":
			const wordsize = 32 << (^uint(0) >> 32 & 1)
			if wordsize == 64 {
				cliFileName = "s390x64_odbc_cli.tar.gz"
			} else {
				cliFileName = "s390_odbc_cli.tar.gz"
			}
		}
	case "aix":
		unpackageType = 3
		const wordsize = 32 << (^uint(0) >> 32 & 1)
		if wordsize == 64 {
			cliFileName = "aix64_odbc_cli.tar.gz"
		} else {
			cliFileName = "aix32_odbc_cli.tar.gz"
		}
	case "sunos":
		unpackageType = 2
		switch runtime.GOARCH {
		case "i86pc":
			const wordsize = 32 << (^uint(0) >> 32 & 1)
			if wordsize == 64 {
				cliFileName = "sunamd64_odbc_cli.tar.gz"
			} else {
				cliFileName = "sunamd32_odbc_cli.tar.gz"
			}
		case "SUNW":
			const wordsize = 32 << (^uint(0) >> 32 & 1)
			if wordsize == 64 {
				cliFileName = "sun64_odbc_cli.tar.gz"
			} else {
				cliFileName = "sun32_odbc_cli.tar.gz"
			}
		}
	case "darwin":
		unpackageType = 2
		const wordsize = 32 << (^uint(0) >> 32 & 1)
		if wordsize == 64 {
			if runtime.GOARCH == "arm64" {
				cliFileName = "macarm64_odbc_cli.tar.gz"
			} else {
				cliFileName = "macos64_odbc_cli.tar.gz"
			}
		}
	default:
		fmt.Println("Error: Unsupported platform!")
		os.Exit(3)
	}
	fileURL := getDownloadURL(cliFileName)
	fmt.Println("Downloading " + fileURL)

	// Try primary
	err := downloadFile(fileURL, cliFileName)
	if err == nil {
		fmt.Println("Download completed.")
		return
	}
	fmt.Println("Error:", err)
	// Try fallback if IBM_DB_DOWNLOAD_URL is not set.
	downloadURL, downloadURLFound := os.LookupEnv("IBM_DB_DOWNLOAD_URL")
	if downloadURLFound && downloadURL != "" {
		os.Exit(5)
	}
	fmt.Println("Trying fallback URL...")
	newURL := strings.ReplaceAll(fileURL, primaryURL, fallbackURL)
	err = downloadFile(newURL, cliFileName)
	if err != nil {
		fmt.Println("Fallback download failed:", err)
		fmt.Println("Error while downloading file: " + err.Error())
		os.Exit(4)
	}
	fmt.Println("Download completed.")

	switch unpackageType {
	case 1:
		Unzipping(cliFileName, target)
	case 3:
		if err := aixUntar(cliFileName, target); err != nil {
			fmt.Println("Error extracting archive:", err)
		}
	default:
		if err := linuxUntar(cliFileName, target, ibmdbDir); err != nil {
			fmt.Println("Error extracting archive:", err)
		}
	}
}

func getDownloadURL(cliFileName string) string {
	downloadURL, downloadURLFound := os.LookupEnv("IBM_DB_DOWNLOAD_URL")
	if !downloadURLFound || downloadURL == "" {
		clidriverVersion, ok := os.LookupEnv("CLIDRIVER_DOWNLOAD_VERSION")
		if ok && clidriverVersion != "" {
			switch {
			case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" && strings.HasPrefix(strings.ToLower(clidriverVersion), "v11"):
				downloadURL = primaryURL + cliFileName
			case runtime.GOOS == "darwin" && runtime.GOARCH != "arm64" && strings.HasPrefix(strings.ToLower(clidriverVersion), "v12"):
				downloadURL = primaryURL + cliFileName
			default:
				downloadURL = primaryURL + clidriverVersion + "/" + cliFileName
			}
		} else {
			downloadURL = primaryURL + cliFileName
		}
	} else {
		fmt.Println("IBM_DB_DOWNLOAD_URL is set to", downloadURL)
	}
	return downloadURL
}
