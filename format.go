package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func formatDoc() {
	inputFile := "posts.txt"
	content, err := readLines(inputFile)
	if err != nil {
		logger.Error("Error reading file: %v", err)
		return
	}

	formattedPosts := formatMultiplePosts(content)
	finalContent := strings.Join(formattedPosts, "\n\n")

	err = os.WriteFile(inputFile, []byte(finalContent), 0644)
	if err != nil {
		logger.Error("Error writing file: %v", err)
		return
	}

	logger.Info("File formatted and overwritten successfully.")
}

func readLines(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func formatMultiplePosts(content []string) []string {
	validCategories := map[string]bool{
		"icpnl": true, "fiscal": true, "laboral": true, "comercio-exterior": true,
		"nacional": true, "empresas": true, "finanzas": true, "rse": true,
	}

	var formattedPosts []string
	var category, title, postContent string
	var isContent bool

	for _, rawLine := range content {
		line := strings.TrimSpace(rawLine)

		if line == "" {
			if isContent && postContent != "" {
				formattedPosts = append(formattedPosts, formatPost(category, title, postContent))
				title = ""
				postContent = ""
				isContent = false
			}
			continue
		}

		if validCategories[line] {
			if category != "" && isContent {
				formattedPosts = append(formattedPosts, formatPost(category, title, postContent))
				title = ""
				postContent = ""
				isContent = false
			}
			category = line
		} else if category != "" && title == "" {
			title = line
		} else if category != "" && title != "" {
			isContent = true
			postContent += line + "\n"
		}
	}

	if isContent && postContent != "" {
		formattedPosts = append(formattedPosts, formatPost(category, title, postContent))
	}

	return formattedPosts
}

func formatPost(category, title, content string) string {
	return fmt.Sprintf("Title: %s\nCategory: %s\nImage: \n%s", title, category, strings.TrimSpace(content))
}
