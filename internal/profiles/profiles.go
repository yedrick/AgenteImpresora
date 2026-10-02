package profiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Profile struct {
	Name             string `json:"name"`
	PaperWidth       int    `json:"paper_width"`
	SupportsQR       bool   `json:"supports_qr"`
	SupportsImages   bool   `json:"supports_images"`
	SupportsBarcodes bool   `json:"supports_barcodes"`
	Codepage         string `json:"codepage"`
}

type Store struct {
	Profiles map[string]Profile
}

func LoadDir(dir string) (*Store, error) {
	store := &Store{Profiles: map[string]Profile{}}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return store, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return store, err
		}
		var profile Profile
		if err := json.Unmarshal(b, &profile); err != nil {
			return store, err
		}
		key := strings.TrimSuffix(strings.ToLower(entry.Name()), ".json")
		if profile.Name == "" {
			profile.Name = key
		}
		store.Profiles[key] = profile
	}
	return store, nil
}

func (s *Store) Get(name string) (Profile, bool) {
	if s == nil {
		return Profile{}, false
	}
	p, ok := s.Profiles[strings.ToLower(name)]
	return p, ok
}
