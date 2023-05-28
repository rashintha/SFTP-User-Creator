package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/rashintha/logger"
	"github.com/xuri/excelize/v2"
)

func main() {

	logger.Defaultln("Opening file: users_list.xlsx")
	f, err := excelize.OpenFile("users_list.xlsx")
	if err != nil {
		logger.ErrorFatal(err.Error())
	}

	logger.Defaultln("Reading file: users_list.xlsx")
	rows, err := f.GetRows("Users")
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

		// Print the output
		fmt.Println(strings.TrimSuffix(string(output), "\n"))
	}
}
