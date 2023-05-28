package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/rashintha/logger"
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

	logger.Defaultln("Validating the content.")
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

		// Print the output
		fmt.Println(strings.TrimSuffix(string(output), "\n"))
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
