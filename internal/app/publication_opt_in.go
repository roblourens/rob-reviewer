package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/roblourens/rob-reviewer/internal/config"
	"github.com/roblourens/rob-reviewer/internal/github"
	"github.com/roblourens/rob-reviewer/internal/review"
)

type optInContentReader interface {
	GetContent(context.Context, string, string, string, string) (github.FileContent, bool, error)
}

func publicationOptInCheck(client optInContentReader, cfg *config.OptInConfig) review.PublicationCheck {
	if cfg == nil {
		return nil
	}
	return func(ctx context.Context, pull review.PullRequest) error {
		file, exists, err := client.GetContent(ctx, cfg.Owner, cfg.Repo, cfg.Path, cfg.Branch)
		if err != nil {
			return fmt.Errorf("read publication opt-in file: %w", err)
		}
		if !exists {
			return errors.New("publication opt-in file is missing or inaccessible")
		}
		optIns, err := decodePublicationOptIns(file.Content)
		if err != nil {
			return err
		}
		if !optIns[strings.ToLower(pull.AuthorLogin)] {
			return fmt.Errorf("%w: PR author has not opted in", review.ErrPublicationSuppressed)
		}
		return nil
	}
}

func decodePublicationOptIns(content []byte) (map[string]bool, error) {
	if len(content) > 64<<10 {
		return nil, errors.New("publication opt-in file exceeds 64 KiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("publication opt-in file must be a JSON object of username-to-boolean entries")
	}
	optIns := make(map[string]bool)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, errors.New("invalid publication opt-in username entry")
		}
		username, ok := key.(string)
		if !ok || username == "" || strings.TrimSpace(username) != username {
			return nil, errors.New("publication opt-in usernames must be nonempty and have no surrounding whitespace")
		}
		username = strings.ToLower(username)
		if _, exists := optIns[username]; exists {
			return nil, errors.New("publication opt-in file contains a duplicate username")
		}
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, errors.New("invalid publication opt-in value")
		}
		enabled, ok := value.(bool)
		if !ok {
			return nil, errors.New("publication opt-in values must be booleans")
		}
		optIns[username] = enabled
	}
	if _, err := decoder.Token(); err != nil {
		return nil, errors.New("publication opt-in file has an invalid closing delimiter")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("publication opt-in file contains trailing content")
	}
	return optIns, nil
}
