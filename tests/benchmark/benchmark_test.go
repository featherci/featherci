package benchmark_test

import (
	"testing"
	"time"

	"github.com/featherci/featherci/internal/crypto"
	"github.com/featherci/featherci/internal/graph"
	"github.com/featherci/featherci/internal/models"
	"github.com/featherci/featherci/internal/workflow"
)

var workflowYAML = []byte(`
name: CI Pipeline
on:
  push:
    branches: [main]
steps:
  - name: lint
    image: golangci/golangci-lint:latest
    commands:
      - golangci-lint run ./...

  - name: test
    image: golang:1.22
    commands:
      - go test -race ./...
    depends_on: [lint]

  - name: build
    image: golang:1.22
    commands:
      - go build -o app ./cmd/server
    depends_on: [test]

  - name: deploy
    image: alpine:latest
    commands:
      - echo "deploying"
    depends_on: [build]
`)

func BenchmarkWorkflowParsing(b *testing.B) {
	parser := workflow.NewParser()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parser.Parse(workflowYAML)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGraphLayout(b *testing.B) {
	now := time.Now()
	finished := now.Add(5 * time.Second)

	steps := []*models.BuildStep{
		{ID: 1, Name: "lint", Status: models.StepStatusSuccess, StartedAt: &now, FinishedAt: &finished},
		{ID: 2, Name: "test", Status: models.StepStatusSuccess, DependsOn: []string{"lint"}, StartedAt: &now, FinishedAt: &finished},
		{ID: 3, Name: "build", Status: models.StepStatusRunning, DependsOn: []string{"test"}, StartedAt: &now},
		{ID: 4, Name: "deploy", Status: models.StepStatusPending, DependsOn: []string{"build"}},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		layout := graph.Calculate(steps)
		if layout == nil {
			b.Fatal("expected non-nil layout")
		}
	}
}

func BenchmarkEncryption(b *testing.B) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	enc, err := crypto.NewEncryptor(key)
	if err != nil {
		b.Fatal(err)
	}

	plaintext := []byte("super-secret-api-key-12345")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ciphertext, err := enc.Encrypt(plaintext)
		if err != nil {
			b.Fatal(err)
		}
		_, err = enc.Decrypt(ciphertext)
		if err != nil {
			b.Fatal(err)
		}
	}
}
