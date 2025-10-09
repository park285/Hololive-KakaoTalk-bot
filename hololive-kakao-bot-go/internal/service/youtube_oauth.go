package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

const (
	tokenFile       = "token.json"
	credentialsFile = "credentials.json"
)

type YouTubeOAuthService struct {
	service *youtube.Service
	config  *oauth2.Config
	token   *oauth2.Token
	logger  *zap.Logger
}

func NewYouTubeOAuthService(logger *zap.Logger) (*YouTubeOAuthService, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	credBytes, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("unable to read credentials file: %w", err)
	}

	config, err := google.ConfigFromJSON(credBytes, youtube.YoutubeReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse credentials: %w", err)
	}

	token, err := loadToken(tokenFile)
	if err != nil {
		logger.Warn("No existing token found, need to authorize",
			zap.String("file", tokenFile))

		return &YouTubeOAuthService{
			config: config,
			token:  nil,
			logger: logger,
		}, nil
	}

	ctx := context.Background()
	client := config.Client(ctx, token)

	ytService, err := youtube.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("failed to create YouTube service: %w", err)
	}

	logger.Info("YouTube OAuth service initialized",
		zap.Bool("authenticated", true))

	return &YouTubeOAuthService{
		service: ytService,
		config:  config,
		token:   token,
		logger:  logger,
	}, nil
}

func (ys *YouTubeOAuthService) Authorize(ctx context.Context) error {
	if ys == nil {
		return fmt.Errorf("service not initialized")
	}

	authURL := ys.config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

	ys.logger.Info("Authorization required")
	fmt.Println("\n=== YouTube API Authorization ===")
	fmt.Println("Go to the following link in your browser:")
	fmt.Println(authURL)
	fmt.Println("\nAfter authorization, enter the code here:")

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		return fmt.Errorf("unable to read authorization code: %w", err)
	}

	token, err := ys.config.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("unable to retrieve token: %w", err)
	}

	if err := saveToken(tokenFile, token); err != nil {
		return fmt.Errorf("unable to save token: %w", err)
	}

	ys.token = token

	client := ys.config.Client(ctx, token)
	ytService, err := youtube.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return fmt.Errorf("failed to create YouTube service: %w", err)
	}

	ys.service = ytService

	ys.logger.Info("YouTube OAuth authorization complete",
		zap.String("token_file", tokenFile))

	fmt.Println("\n✅ Authorization successful! Token saved.")

	return nil
}

func (ys *YouTubeOAuthService) IsAuthorized() bool {
	return ys != nil && ys.service != nil && ys.token != nil
}

func (ys *YouTubeOAuthService) GetService() *youtube.Service {
	if ys == nil {
		return nil
	}
	return ys.service
}

func loadToken(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	token := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(token)
	return token, err
}

func saveToken(file string, token *oauth2.Token) error {
	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(token)
}
