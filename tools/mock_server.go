package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"
)

type Article struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	PublishedAt string `json:"publishedAt"`
}

type NewsResponse struct {
	Articles []Article `json:"articles"`
}

var headlines = []string{
	"Breaking: New Go Version Released with Performance Improvements",
	"Kubernetes 1.30 Brings Enhanced Security Features",
	"Docker Updates Container Runtime for Better Resource Management",
	"Cloud Computing Trends: Multi-Cloud Strategies Gain Popularity",
	"AI Integration in DevOps: Automating Infrastructure Management",
	"Microservices Architecture: Best Practices for 2025",
	"Cyber Security Alert: New Vulnerabilities in Popular Frameworks",
	"Database Performance: Optimizing Queries for Large Scale Applications",
	"Web Development: Progressive Web Apps See Increased Adoption",
	"Mobile Development: Cross-Platform Solutions Comparison",
}

var descriptions = []string{
	"This breakthrough announcement changes the landscape of modern software development...",
	"Industry experts weigh in on the implications for deployments...",
	"A comprehensive analysis of the new features and their potential impact...",
	"Early adopters report significant improvements in performance and reliability...",
	"The development community responds positively to these latest changes...",
	"Security researchers highlight the importance of upgrading immediately...",
	"Benchmarks show remarkable improvements in speed and efficiency...",
	"This update addresses several long-standing issues reported by users...",
	"Leading companies share their migration strategies and lessons learned...",
	"Detailed documentation and tutorials help developers get started quickly...",
}

func generateRandomNews(count int) []Article {
	articles := make([]Article, count)

	for i := 0; i < count; i++ {
		articles[i] = Article{
			Title:       headlines[rand.Intn(len(headlines))],
			Description: descriptions[rand.Intn(len(descriptions))],
			URL:         fmt.Sprintf("https://example.com/news/article-%d", rand.Intn(10000)),
			PublishedAt: time.Now().Add(-time.Duration(rand.Intn(48)) * time.Hour).Format(time.RFC3339),
		}
	}

	return articles
}

func newsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Генерируем от 3 до 8 новостей
	count := rand.Intn(6) + 3
	articles := generateRandomNews(count)

	response := NewsResponse{
		Articles: articles,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	log.Printf("Served %d articles", len(articles))
}

func main() {
	rand.Seed(time.Now().UnixNano())

	http.HandleFunc("/api/news", newsHandler)

	port := ":3001"
	log.Printf("Mock news server starting on port %s", port)
	log.Printf("Test endpoint: http://localhost%s/api/news", port)

	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
