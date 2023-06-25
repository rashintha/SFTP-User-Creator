package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rashintha/env"
	"github.com/rashintha/logger"
	"github.com/skip2/go-qrcode"
	"github.com/xuri/excelize/v2"
)

func main() {

	frequency, err := strconv.Atoi(env.CONF["FREQUENCY"])
	in_folder := env.CONF["IN"]

	if err != nil {
		logger.ErrorFatal(err.Error())
	}

	ticker := time.NewTicker(time.Duration(frequency) * time.Minute)

	go func() {
		for t := range ticker.C {
			fmt.Println(t)
			files, err := ioutil.ReadDir(in_folder)
			if err != nil {
				logger.Errorln(err.Error())
			}

			if len(files) == 0 {
				logger.Warningf("No files found in the %s directory.", in_folder)
			}

		FilesLoop:
			for _, file := range files {
				fmt.Println(file.Name())

				// Opening file
				logger.Defaultf("Opening file: %v", file.Name())
				f, err := excelize.OpenFile(in_folder + "/" + file.Name())
				if err != nil {
					logger.Errorln(err.Error())
					continue
				}

				logger.Defaultf("Reading file: %v", file.Name())
				rows, err := f.GetRows("Users")
				if err != nil {
					logger.Errorln(err.Error())
					continue
				}

				dirRows, err := f.GetRows("Folders")
				if err != nil {
					logger.Errorln(err.Error())
					continue
				}

				logger.Defaultln("Validating the file content.")
				if rows[0][0] != "Username" {
					logger.Errorln("Header row values are invalid.")
					continue
				}

				for _, row := range rows[1:] {
					if row[0] == "" {
						logger.Errorf("Invalid row entry at %v", row[0])
						continue FilesLoop
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
						logger.Errorln(err.Error())
						continue FilesLoop
					}

					password := generatePassword(12)

					logger.Defaultln("Updating password")
					cmd = exec.Command("bash", "-c", "echo", fmt.Sprintf("'%v:%v'", row[0], password), "|", "sudo", "chpasswd") // replace "ls -l" with your command

					// This will capture the output from the command
					output, err = cmd.CombinedOutput()
					if err != nil {
						logger.Errorln(strings.TrimSuffix(string(output), "\n"))
						logger.Errorln(err.Error())
						continue FilesLoop
					}

					logger.Defaultln("Creating directories")

					err = createDir("files", row[0])
					if err != nil {
						logger.Errorln(err.Error())
						continue FilesLoop
					}

					logger.Defaultln("Changing root directory ownership to root.")
					cmd = exec.Command("sudo", "chown", "root:root", fmt.Sprintf("/var/sftp/%v", row[0])) // replace "ls -l" with your command

					// This will capture the output from the command
					output, err = cmd.CombinedOutput()
					if err != nil {
						logger.Errorln(strings.TrimSuffix(string(output), "\n"))
						logger.Errorln(err.Error())
						continue FilesLoop
					}

					logger.Defaultln("Changing root directory permissions.")
					cmd = exec.Command("sudo", "chmod", "755", fmt.Sprintf("/var/sftp/%v", row[0])) // replace "ls -l" with your command

					// This will capture the output from the command
					output, err = cmd.CombinedOutput()
					if err != nil {
						logger.Errorln(strings.TrimSuffix(string(output), "\n"))
						logger.Errorln(err.Error())
						continue FilesLoop
					}

					for _, dirRow := range dirRows {
						err = createDir(dirRow[0], row[0])
						if err != nil {
							logger.Errorln(err.Error())
							continue FilesLoop
						}
					}

					secret, _, err := googleAuthenticator(row[0])
					if err != nil {
						logger.Errorln(err.Error())
						continue FilesLoop
					}

					err = prepareDoc(row[0], password, secret)
					if err != nil {
						logger.Errorln(err.Error())
						continue FilesLoop
					}
				}
			}
		}
	}()

	select {}

	// if len(os.Args) < 2 {
	// 	logger.ErrorFatal("File name is not provided.")
	// }

	// fileName := os.Args[1]

}

func prepareDoc(user string, password string, secret string) error {
	logger.Defaultf("Preparing word document")

	logger.Defaultf("Removing existing doc processing folders")

	err := os.RemoveAll("doc")
	if err != nil {
		return err
	}

	logger.Defaultf("Unzipping the sample document.")
	cmd := exec.Command("unzip", "-d", "doc", "doc.docx")

	err = cmd.Run()
	if err != nil {
		return err
	}

	logger.Defaultf("Copying qr code to document")

	// Open source file for reading
	srcFile, err := os.Open(fmt.Sprintf("qr/%v.png", user))
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Open destination file for writing
	dstFile, err := os.Create("doc/word/media/image2.png")
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// Copy the source file to the destination file
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}

	// Sync to ensure that the copy operation is complete before the program exits
	err = dstFile.Sync()
	if err != nil {
		return err
	}

	logger.Defaultf("Updating the document")

	docPath := "doc/word/document.xml"
	// Read the file
	content, err := ioutil.ReadFile(docPath)
	if err != nil {
		return err
	}

	// Replace the phrase
	newContent := strings.ReplaceAll(string(content), "&lt;user&gt;", user)
	newContent = strings.ReplaceAll(string(newContent), "&lt;pass&gt;", password)
	newContent = strings.ReplaceAll(string(newContent), "&lt;secret&gt;", secret)

	// Write the new content back to the file
	err = ioutil.WriteFile(docPath, []byte(newContent), 0644)
	if err != nil {
		return err
	}

	out_dir := env.CONF["OUT"]

	logger.Defaultf("Creating OUT directory if not exists")
	cmd = exec.Command("sudo", "mkdir", "-p", out_dir) // replace "ls -l" with your command

	// This will capture the output from the command
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Errorln(strings.TrimSuffix(string(output), "\n"))
		return err
	}

	logger.Defaultf("Creating the updated document")

	// Create a file to write our archive to.
	file, err := os.Create(fmt.Sprintf("%v/%v.docx", out_dir, user))
	if err != nil {
		return err
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
		return err
	}

	// Make sure to check the error on Close.
	err = w.Close()
	if err != nil {
		return err
	}

	return nil
}

func createDir(dir string, user string) error {
	dirPath := fmt.Sprintf("/var/sftp/%v/%v", user, dir)

	logger.Defaultf("Creating directory: %v", dirPath)
	cmd := exec.Command("sudo", "mkdir", "-p", dirPath) // replace "ls -l" with your command

	// This will capture the output from the command
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Errorln(strings.TrimSuffix(string(output), "\n"))
		return err
	}

	logger.Defaultf("Changing directory ownership to %v.", user)
	cmd = exec.Command("sudo", "chown", fmt.Sprintf("%v:sftpusers", user), dirPath) // replace "ls -l" with your command

	// This will capture the output from the command
	output, err = cmd.CombinedOutput()
	if err != nil {
		logger.Errorln(strings.TrimSuffix(string(output), "\n"))
		return err
	}

	logger.Defaultln("Changing root directory permissions.")
	cmd = exec.Command("sudo", "chmod", "700", dirPath) // replace "ls -l" with your command

	// This will capture the output from the command
	output, err = cmd.CombinedOutput()
	if err != nil {
		logger.Errorln(strings.TrimSuffix(string(output), "\n"))
		return err
	}

	return nil
}

func generatePassword(length int) string {
	rand.Seed(time.Now().UnixNano())
	digits := "0123456789"
	specials := "~=+%^*/()[]{}/!@#$?|"
	all := "ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		digits + specials
	buf := make([]byte, length)
	for i := range buf {
		buf[i] = all[rand.Intn(len(all))]
	}
	return string(buf)
}

func googleAuthenticator(user string) (string, []string, error) {
	logger.Defaultln("Enabling Google authenticator")

	cmd := exec.Command("sudo", "-u", user, "google-authenticator", "-t", "-d", "-r3", "-R30", "-f", "-C", "-w3", "-Q", "UTF8") // replace "ls -l" with your command

	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Errorln(strings.TrimSuffix(string(output), "\n"))
		return "", nil, err
	}

	logger.Defaultf("Generating the QR Code")
	outStr := string(output)
	startIndex := strings.Index(outStr, "otpauth://totp/")
	endIndex := strings.Index(outStr[startIndex:], "\n")

	if startIndex == -1 || endIndex == -1 {
		return "", nil, errors.New("Could not find URL")
	}

	url := outStr[startIndex : startIndex+endIndex]

	var png []byte
	png, err = qrcode.Encode(url, qrcode.Medium, 1024)
	if err != nil {
		return "", nil, err
	}

	logger.Defaultf("Creating qr directory if not exists")
	cmd = exec.Command("sudo", "mkdir", "-p", "qr") // replace "ls -l" with your command

	// This will capture the output from the command
	output, err = cmd.CombinedOutput()
	if err != nil {
		logger.Errorln(strings.TrimSuffix(string(output), "\n"))
		return "", nil, err
	}

	logger.Defaultf("Saving the QR code")
	qrFileName := fmt.Sprintf("qr/%v.png", user)

	// Write the PNG to a file
	err = ioutil.WriteFile(qrFileName, png, 0644)
	if err != nil {
		return "", nil, err
	}

	startIndex = strings.Index(outStr, "Your new secret key is: ") + 24
	endIndex = strings.Index(outStr[startIndex:], "\n")

	if startIndex == -1 || endIndex == -1 {
		return "", nil, errors.New("Could not find the secret key.")
	}

	secretKey := outStr[startIndex : startIndex+endIndex]

	// Find the scratch codes
	startIndex = strings.Index(outStr, "Your emergency scratch codes are:")
	if startIndex == -1 {
		return "", nil, errors.New("Could not find scratch codes")
	}
	scratchCodes := strings.Split(outStr[startIndex:], "\n")[1:6]

	return secretKey, scratchCodes, nil
}
