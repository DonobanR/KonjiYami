package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"konjiyami/server/assets"
	"golang.org/x/crypto/bcrypt"
)

// Operator represents a KonjiYami operator with RBAC role.
type Operator struct {
	Username string `json:"username"`
	Hash     string `json:"hash"`
	Role     string `json:"role"` // admin, operator, viewer
}

type operatorStore struct {
	mu       sync.RWMutex
	filePath string
	ops      []Operator
}

func newOperatorStore() *operatorStore {
	return &operatorStore{
		filePath: filepath.Join(assets.GetRootAppDir(), "operators.json"),
	}
}

func (st *operatorStore) init() error {
	st.mu.Lock()
	defer st.mu.Unlock()

	data, err := os.ReadFile(st.filePath)
	if err == nil {
		return json.Unmarshal(data, &st.ops)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("konjiyami"), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash default password: %w", err)
	}
	st.ops = []Operator{{
		Username: "admin",
		Hash:     string(hash),
		Role:     "admin",
	}}
	return st.saveLocked()
}

func (st *operatorStore) saveLocked() error {
	data, err := json.MarshalIndent(st.ops, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(st.filePath, data, 0600)
}

func (st *operatorStore) authenticate(username, password string) (*Operator, error) {
	st.mu.RLock()
	defer st.mu.RUnlock()

	for i := range st.ops {
		if st.ops[i].Username == username {
			if err := bcrypt.CompareHashAndPassword([]byte(st.ops[i].Hash), []byte(password)); err != nil {
				return nil, fmt.Errorf("invalid credentials")
			}
			op := st.ops[i]
			return &op, nil
		}
	}
	return nil, fmt.Errorf("invalid credentials")
}

func (st *operatorStore) list() []Operator {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]Operator, len(st.ops))
	copy(out, st.ops)
	return out
}

func isValidRole(role string) bool {
	return role == "admin" || role == "operator" || role == "viewer"
}
