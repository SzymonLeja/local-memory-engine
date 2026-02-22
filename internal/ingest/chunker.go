package ingest

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

const (
	MaxTokens   = 512
	OverlapSize = 50
)

type Chunk struct {
	ID       string
	Text     string
	Position string
}

func estimateTokens(text string) int {
	return len(text) / 4
}

func chunkID(filePath, text string) string {
	raw := filePath + text
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", sum)
}

func ChunkText(filePath, content string) []Chunk {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	paragraphs := strings.Split(content, "\n\n")
	var chunks []Chunk
	i := 0

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		if estimateTokens(para) <= MaxTokens {
			chunks = append(chunks, Chunk{
				ID:       chunkID(filePath, para),
				Text:     para,
				Position: fmt.Sprintf("paragraph %d", i+1),
			})
			i++
		} else {
			maxChars := MaxTokens * 4
			overlapChars := OverlapSize * 4
			runes := []rune(para)

			for start := 0; start < len(runes); start += maxChars - overlapChars {
				end := start + maxChars
				if end > len(runes) {
					end = len(runes)
				}

				window := strings.TrimSpace(string(runes[start:end]))
				if window == "" {
					break
				}

				chunks = append(chunks, Chunk{
					ID:       chunkID(filePath, window),
					Text:     window,
					Position: fmt.Sprintf("paragraph %d (window %d)", i+1, start/maxChars+1),
				})

				if end == len(runes) {
					break
				}
			}
			i++
		}
	}

	return chunks
}
