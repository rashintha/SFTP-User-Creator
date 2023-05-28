package main

import (
	"archive/zip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rashintha/logger"
	"github.com/skip2/go-qrcode"
	"github.com/xuri/excelize/v2"
)

func main() {

	if len(os.Args) < 2 {
		logger.ErrorFatal("File name is not provided.")
	}

	fileName := os.Args[1]

	// Opening file
	logger.Defaultf("Opening file: %v", fileName)
	f, err := excelize.OpenFile(fileName)
	if err != nil {
		logger.ErrorFatal(err.Error())
	}

	logger.Defaultf("Reading file: %v", fileName)
	rows, err := f.GetRows("Users")
	if err != nil {
		logger.ErrorFatal(err.Error())
	}

	dirRows, err := f.GetRows("Folders")
	if err != nil {
		logger.ErrorFatal(err.Error())
	}

	logger.Defaultln("Validating the file content.")
	if rows[0][0] != "Username" || rows[0][1] != "Password" {
		logger.ErrorFatal("Header row values are invalid.")
	}

	for _, row := range rows[1:] {
		if row[0] == "" || row[1] == "" {
			logger.ErrorFatalf("Invalid row entry at %v : %v", row[0], row[1])
		}
	}

	logger.Defaultln("Creating SFTP users.")

	for _, row := range rows[1:] {
		logger.Defaultf("Creating user: %v", row[0])
		cmd := exec.Command("sudo", "useradd", "-m", row[0], "-g", "sftpusers") // replace "ls -l" with your command

		// This will capture the output from the command
		output, err := cmd.CombinedOutput()
		if err != nil {
			logger.Errorln(strings.TrimSuffix(string(output), "\n"))
			logger.ErrorFatal(err.Error())
		}

		logger.Defaultln("Updating password")
		cmd = exec.Command("bash", "-c", "echo", fmt.Sprintf("'%v:%v'", row[0], row[1]), "|", "sudo", "chpasswd") // replace "ls -l" with your command

		// This will capture the output from the command
		output, err = cmd.CombinedOutput()
		if err != nil {
			logger.Errorln(strings.TrimSuffix(string(output), "\n"))
			logger.ErrorFatal(err.Error())
		}

		logger.Defaultln("Creating directories")

		createDir("files", row[0])

		logger.Defaultln("Changing root directory ownership to root.")
		cmd = exec.Command("sudo", "chown", "root:root", fmt.Sprintf("/var/sftp/%v", row[0])) // replace "ls -l" with your command

		// This will capture the output from the command
		output, err = cmd.CombinedOutput()
		if err != nil {
			logger.Errorln(strings.TrimSuffix(string(output), "\n"))
			logger.ErrorFatal(err.Error())
		}

		logger.Defaultln("Changing root directory permissions.")
		cmd = exec.Command("sudo", "chmod", "755", fmt.Sprintf("/var/sftp/%v", row[0])) // replace "ls -l" with your command

		// This will capture the output from the command
		output, err = cmd.CombinedOutput()
		if err != nil {
			logger.Errorln(strings.TrimSuffix(string(output), "\n"))
			logger.ErrorFatal(err.Error())
		}

		for _, dirRow := range dirRows {
			createDir(dirRow[0], row[0])
		}

		secret, _ := googleAuthenticator(row[0])

		prepareDoc(row[0], row[1], secret)

	}
}

func prepareDoc(user string, password string, secret string) {
	logger.Defaultf("Preparing word document")

	logger.Defaultf("Removing existing doc processing folders")

	err := os.RemoveAll("doc")
	if err != nil {
		logger.Errorf("Failed removing directory: %s", err)
	}

	logger.Defaultf("Unzipping the sample document.")
	cmd := exec.Command("unzip", "-d", "doc", "doc.docx")

	err = cmd.Run()
	if err != nil {
		logger.ErrorFatalf("Unzip command failed with %s\n", err)
	}

	logger.Defaultf("Copying qr code to document")

	// Open source file for reading
	srcFile, err := os.Open(fmt.Sprintf("qr/%v.png", user))
	if err != nil {
		logger.ErrorFatal(err.Error())
	}
	defer srcFile.Close()

	// Open destination file for writing
	dstFile, err := os.Create("doc/word/media/image2.png")
	if err != nil {
		logger.ErrorFatal(err.Error())
	}
	defer dstFile.Close()

	// Copy the source file to the destination file
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		logger.ErrorFatal(err.Error())
	}

	// Sync to ensure that the copy operation is complete before the program exits
	err = dstFile.Sync()
	if err != nil {
		logger.ErrorFatal(err.Error())
	}

	logger.Defaultf("Updating the document")

	docPath := "doc/word/document.xml"
	// Read the file
	content, err := ioutil.ReadFile(docPath)
	if err != nil {
		logger.ErrorFatalf("Error reading file: %s", err)
	}

	// Replace the phrase
	newContent := strings.ReplaceAll(string(content), "&lt;user&gt;", user)
	newContent = strings.ReplaceAll(string(newContent), "&lt;pass&gt;", password)
	newContent = strings.ReplaceAll(string(newContent), "&lt;secret&gt;", secret)

	// Write the new content back to the file
	err = ioutil.WriteFile(docPath, []byte(newContent), 0644)
	if err != nil {
		logger.ErrorFatalf("Error writing file: %s", err)
	}

	logger.Defaultf("Creating documents directory if not exists")
	cmd = exec.Command("sudo", "mkdir", "-p", "documents") // replace "ls -l" with your command

	// This will capture the output from the command
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Errorln(strings.TrimSuffix(string(output), "\n"))
		logger.ErrorFatal(err.Error())
	}

	logger.Defaultf("Creating the updated document")

	// Create a file to write our archive to.
	file, err := os.Create(fmt.Sprintf("documents/%v.docx", user))
	if err != nil {
		logger.ErrorFatalf("Error creating docx file: %s", err)
	}
	defer file.Close()

	// Create a new zip archive.
	w := zip.NewWriter(file)

	// Add files to zip.
	if err := filepath.Walk("doc", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Create header.
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		header.Name = strings.TrimPrefix(path, filepath.Clean("doc")+"/")

		if info.IsDir() {
			header.Name += "/"
		} else {
			// If file is a symlink, dereference it.
			if info.Mode()&os.ModeSymlink != 0 {
				realPath, err := os.Readlink(path)
				if err != nil {
					return err
				}
				header.Method = zip.Deflate
				path = realPath
			}
		}

		// Create writer.
		writer, err := w.CreateHeader(header)
		if err != nil {
			return err
		}

		// Write data to zip.
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			_, err = io.Copy(writer, file)
			file.Close()
			if err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		logger.ErrorFatalf("Error writing file: %s", err)
	}

	// Make sure to check the error on Close.
	err = w.Close()
	if err != nil {
		logger.ErrorFatalf("Error closing writer: %s", err)
	}
}

func createDir(dir string, user string) {
	dirPath := fmt.Sprintf("/var/sftp/%v/%v", user, dir)

	logger.Defaultf("Creating directory: %v", dirPath)
	cmd := exec.Command("sudo", "mkdir", "-p", dirPath) // replace "ls -l" with your command

	// This will capture the output from the command
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Errorln(strings.TrimSuffix(string(output), "\n"))
		logger.ErrorFatal(err.Error())
	}

	logger.Defaultf("Changing directory ownership to %v.", user)
	cmd = exec.Command("sudo", "chown", fmt.Sprintf("%v:sftpusers", user), dirPath) // replace "ls -l" with your command

	// This will capture the output from the command
	output, err = cmd.CombinedOutput()
	if err != nil {
		logger.Errorln(strings.TrimSuffix(string(output), "\n"))
		logger.ErrorFatal(err.Error())
	}

	logger.Defaultln("Changing root directory permissions.")
	cmd = exec.Command("sudo", "chmod", "700", dirPath) // replace "ls -l" with your command

	// This will capture the output from the command
	output, err = cmd.CombinedOutput()
	if err != nil {
		logger.Errorln(strings.TrimSuffix(string(output), "\n"))
		logger.ErrorFatal(err.Error())
	}
}

func googleAuthenticator(user string) (string, []string) {
	logger.Defaultln("Enabling Google authenticator")

	cmd := exec.Command("sudo", "-u", user, "google-authenticator", "-t", "-d", "-r3", "-R30", "-f", "-C", "-w3", "-Q", "UTF8") // replace "ls -l" with your command

	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Errorln(strings.TrimSuffix(string(output), "\n"))
		logger.ErrorFatal(err.Error())
	}

	logger.Defaultf("Generating the QR Code")
	outStr := string(output)
	startIndex := strings.Index(outStr, "otpauth://totp/")
	endIndex := strings.Index(outStr[startIndex:], "\n")

	if startIndex == -1 || endIndex == -1 {
		logger.ErrorFatal("Could not find URL")

	}

	url := outStr[startIndex : startIndex+endIndex]

	var errQR error
	var png []byte
	png, errQR = qrcode.Encode(url, qrcode.Medium, 1024)
	if errQR != nil {
		logger.ErrorFatalf("Could not generate QR Code: %s\n", errQR)
	}

	logger.Defaultf("Creating qr directory if not exists")
	cmd = exec.Command("sudo", "mkdir", "-p", "qr") // replace "ls -l" with your command

	// This will capture the output from the command
	output, err = cmd.CombinedOutput()
	if err != nil {
		logger.Errorln(strings.TrimSuffix(string(output), "\n"))
		logger.ErrorFatal(err.Error())
	}

	logger.Defaultf("Saving the QR code")
	qrFileName := fmt.Sprintf("qr/%v.png", user)

	// Write the PNG to a file
	err = ioutil.WriteFile(qrFileName, png, 0644)
	if err != nil {
		logger.ErrorFatalf("Could not write file: %s\n", err)
	}

	startIndex = strings.Index(outStr, "Your new secret key is: ") + 24
	endIndex = strings.Index(outStr[startIndex:], "\n")

	if startIndex == -1 || endIndex == -1 {
		logger.ErrorFatal("Could not find the secret key.")

	}

	secretKey := outStr[startIndex : startIndex+endIndex]

	// Find the scratch codes
	startIndex = strings.Index(outStr, "Your emergency scratch codes are:")
	if startIndex == -1 {
		logger.ErrorFatal("Could not find scratch codes")
	}
	scratchCodes := strings.Split(outStr[startIndex:], "\n")[1:6]

	return secretKey, scratchCodes
}
