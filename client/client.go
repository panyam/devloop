package client

import (
	"context"
	"fmt"
	"time"

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client wraps the gRPC client with convenience methods
type Client struct {
	conn   *grpc.ClientConn
	client pb.AgentServiceClient
	ctx    context.Context
}

// Config holds client configuration
type Config struct {
	Address        string
	RequestTimeout time.Duration
}

// NewClient creates a new client instance
func NewClient(cfg Config) (*Client, error) {
	if cfg.Address == "" {
		cfg.Address = "localhost:5555"
	}
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = 30 * time.Second
	}

	conn, err := grpc.NewClient(
		cfg.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to devloop server at %s: %w", cfg.Address, err)
	}

	client := pb.NewAgentServiceClient(conn)
	ctx := context.Background()

	return &Client{
		conn:   conn,
		client: client,
		ctx:    ctx,
	}, nil
}

// Close closes the client connection
func (c *Client) Close() error {
	return c.conn.Close()
}

// GetConfig retrieves the devloop configuration
func (c *Client) GetConfig() (*pb.Config, error) {
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()

	resp, err := c.client.GetConfig(ctx, &pb.GetConfigRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	return resp.Config, nil
}

// GetRuleStatus retrieves the status of a specific rule
func (c *Client) GetRuleStatus(ruleName string) (*pb.Rule, error) {
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()

	resp, err := c.client.GetRule(ctx, &pb.GetRuleRequest{
		RuleName: ruleName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get rule status for %q: %w", ruleName, err)
	}

	return resp.Rule, nil
}

// TriggerRule triggers execution of a specific rule
func (c *Client) TriggerRule(ruleName string) (*pb.TriggerRuleResponse, error) {
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()

	resp, err := c.client.TriggerRule(ctx, &pb.TriggerRuleRequest{
		RuleName: ruleName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to trigger rule %q: %w", ruleName, err)
	}

	return resp, nil
}

// ListWatchedPaths retrieves all watched file patterns
func (c *Client) ListWatchedPaths() ([]string, error) {
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()

	resp, err := c.client.ListWatchedPaths(ctx, &pb.ListWatchedPathsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list watched paths: %w", err)
	}

	return resp.Paths, nil
}

// Ping tests connectivity to the devloop server
func (c *Client) Ping() error {
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	_, err := c.client.GetConfig(ctx, &pb.GetConfigRequest{})
	if err != nil {
		return fmt.Errorf("failed to ping devloop server: %w", err)
	}

	return nil
}