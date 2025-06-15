package main

import (
	"cmp"
	"log/slog"
	"log"
	"os"
	"fmt"
	"net/http"
	"encoding/json"

	"github.com/gin-gonic/gin"

	"github.com/joho/godotenv"

	sloggin "github.com/samber/slog-gin"
)

var validRegions = map[string]struct{}{
    "eu":      {},
    "na":      {},
    "latam":   {},
    "ap":      {},
    "kr":      {},
    "br":      {},
}

func isValidRegion(region string) bool {
    _, ok := validRegions[region]
    return ok
}

func main() {
	env := os.Getenv("ENVIRONMENT")

	if env == "development" {
		err := godotenv.Load()
		
		if err != nil {
			log.Println("Warning: .env file not found")
			return
		}
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	gin.SetMode(gin.ReleaseMode)

	r := gin.New()

	r.Use(sloggin.New(logger))
	r.Use(gin.Recovery())

	r.GET("/rest/v1/rank/:region/:name/:tag", func(c *gin.Context) {
		region := c.Param("region")
		name := c.Param("name")
		tag := c.Param("tag")

		if !isValidRegion(region) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid Region: " + region,
			})
			return
		}

		apiKey := os.Getenv("VALORANT_API_KEY")
		url := fmt.Sprintf("https://api.henrikdev.xyz/valorant/v2/mmr/%s/%s/%s?api_key=%s", region, name, tag, apiKey)
		res, err := http.Get(url)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Issue connecting to external API",
			})
			return
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			c.JSON(res.StatusCode, gin.H{
				"error": fmt.Sprintf("API returned status code: %d", res.StatusCode),
				"url": url,
			})
			return
		}

		var result map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to parse API response",
			})
			return
		}

		// Access current_data from the response
		if data, ok := result["data"].(map[string]interface{}); ok {
			if currentData, ok := data["current_data"].(map[string]interface{}); ok {
				rank, ok := currentData["currenttierpatched"].(string)
				if !ok {
					c.JSON(http.StatusInternalServerError, gin.H{
						"error": "Invalid rank data type",
					})
					return
				}
				rr, ok := currentData["ranking_in_tier"].(float64)
				if !ok {
					c.JSON(http.StatusInternalServerError, gin.H{
						"error": "Invalid RR data type",
					})
					return
				}

				c.JSON(http.StatusOK, gin.H{
					"message": fmt.Sprintf("%s [%dRR]", rank, int(rr)),
				})
				return
			}
		}
	})

	port := cmp.Or(os.Getenv("PORT"), "8080")

	logger.Info("Server starting", slog.String("port", port))

	r.Run((":" + port))
}
