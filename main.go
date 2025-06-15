package main

import (
	"cmp"
	"log/slog"
	"os"
	"fmt"
	"net/http"
	"encoding/json"
	"time"
	"sync"

	"github.com/gin-gonic/gin"

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

type cacheEntry struct {
    data      map[string]interface{}
    timestamp time.Time
}

var (
    cache     = make(map[string]cacheEntry)
    cacheMutex sync.RWMutex
    cacheTTL  = 5 * time.Minute
)

var httpClient = &http.Client{
    Timeout: 10 * time.Second,
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 100,
        IdleConnTimeout:     90 * time.Second,
    },
}

func isValidRegion(region string) bool {
    _, ok := validRegions[region]
    return ok
}

func getFromCache(key string) (map[string]interface{}, bool) {
    cacheMutex.RLock()
    defer cacheMutex.RUnlock()
    
    if entry, exists := cache[key]; exists {
        if time.Since(entry.timestamp) < cacheTTL {
            return entry.data, true
        }
    }
    return nil, false
}

func setCache(key string, data map[string]interface{}) {
    cacheMutex.Lock()
    defer cacheMutex.Unlock()
    
    cache[key] = cacheEntry{
        data:      data,
        timestamp: time.Now(),
    }
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	apiKey := os.Getenv("VALORANT_API_KEY")

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(sloggin.New(logger))
	r.Use(gin.Recovery())

	r.GET("/rest/v1/rank/:region/:name/:tag", func(c *gin.Context) {
		start := time.Now()

		region := c.Param("region")
		name := c.Param("name")
		tag := c.Param("tag")

		if !isValidRegion(region) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid Region: " + region,
			})
			return
		}

		cacheKey := fmt.Sprintf("%s:%s:%s", region, name, tag)

		if cachedData, found := getFromCache(cacheKey); found {
			if currentData, ok := cachedData["current_data"].(map[string]interface{}); ok {
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

				latency := time.Since(start)

				c.JSON(http.StatusOK, gin.H{
					"message": fmt.Sprintf("%s [%dRR]", rank, int(rr)),
					"latency:ms": latency.Milliseconds(),
					"cached": true,
				})
				return
			}
		}

		res, err := httpClient.Get(fmt.Sprintf("https://api.henrikdev.xyz/valorant/v2/mmr/%s/%s/%s?api_key=%s", region, name, tag, apiKey))
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

		if data, ok := result["data"].(map[string]interface{}); ok {
			setCache(cacheKey, data)

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

				latency := time.Since(start)

				c.JSON(http.StatusOK, gin.H{
					"message": fmt.Sprintf("%s [%dRR]", rank, int(rr)),
					"latency:ms": latency.Milliseconds(),
					"cached": false,
				})
				return
			}
		}
	})

	port := cmp.Or(os.Getenv("PORT"), "8080")

	logger.Info("Server starting", slog.String("port", port))

	r.Run((":" + port))
}
