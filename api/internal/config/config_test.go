package config

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	if _, err := load(env(nil)); err == nil {
		t.Fatal("want error without DATABASE_URL")
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	c, err := load(env(map[string]string{"DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPAddr != "127.0.0.1:8080" || c.DecisionModel != "typesafe/jev-1.13" {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestStringHidesKey(t *testing.T) {
	c, _ := load(env(map[string]string{"DATABASE_URL": "postgres://x", "OPENROUTER_API_KEY": "sk-secret"}))
	if strings.Contains(c.String(), "sk-secret") {
		t.Fatal("String leaks the API key")
	}
}

func TestLoadAppliesThePhotoReaderDefaults(t *testing.T) {
	c, err := load(env(map[string]string{"DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.PhotoReader != "jev" || c.VisionModel != "google/gemini-3.1-flash-lite" || c.PhotoJevModel != "typesafe/jev-1.14" || c.DataDir != "data" {
		t.Fatalf("photo defaults: reader %q, vision %q, jev %q, data %q", c.PhotoReader, c.VisionModel, c.PhotoJevModel, c.DataDir)
	}
}

func TestLoadRejectsAnUnknownPhotoReader(t *testing.T) {
	if _, err := load(env(map[string]string{"DATABASE_URL": "postgres://x", "PHOTO_READER": "ocr"})); err == nil {
		t.Fatal("PHOTO_READER=ocr accepted")
	}
}
