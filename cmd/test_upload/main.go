package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	appData := os.Getenv("APPDATA")
	installDir := filepath.Join(appData, "MonitoringAgent")
	filePath := filepath.Join(installDir, "data", "screenshots", "screenshot_20260801_151349.jpg")

	fileData, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("screenshot", filepath.Base(filePath))
	if err != nil {
		fmt.Println("Error creating form file:", err)
		return
	}
	_, _ = part.Write(fileData)

	capturedAt := time.Now().Format(time.RFC3339)
	metadataJSON, _ := json.Marshal(map[string]interface{}{
		"timestamp":   capturedAt,
		"activeApp":   "Desktop",
		"windowTitle": "Desktop Capture",
	})
	_ = writer.WriteField("metadata", string(metadataJSON))
	_ = writer.Close()

	apiURL := "http://127.0.0.1:8000/api"
	agentID := "eeae7190-6a37-4aeb-ad1a-24db8c645a5a"
	apiKey := "f5deb1bc-360b-411c-82d5-6887a3c4fd41"

	uploadURL := fmt.Sprintf("%s/agents/%s/uploads", apiURL, agentID)
	fmt.Println("Posting to:", uploadURL)

	req, err := http.NewRequest(http.MethodPost, uploadURL, body)
	if err != nil {
		fmt.Println("Request error:", err)
		return
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("HTTP client error:", err)
		return
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	fmt.Printf("Status: %d | Response: %s\n", resp.StatusCode, string(bodyBytes))
}
