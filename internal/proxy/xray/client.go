package xray

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	proxymanCommand "github.com/xtls/xray-core/app/proxyman/command"
	statsCommand "github.com/xtls/xray-core/app/stats/command"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/proxy/vless"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	conn        *grpc.ClientConn
	proxyman    proxymanCommand.HandlerServiceClient
	stats       statsCommand.StatsServiceClient
}

func NewClient(addr string, useTLS bool) (*Client, error) {
	opts := []grpc.DialOption{}
	if !useTLS {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("xray grpc dial %s: %w", addr, err)
	}

	return &Client{
		conn:     conn,
		proxyman: proxymanCommand.NewHandlerServiceClient(conn),
		stats:    statsCommand.NewStatsServiceClient(conn),
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

// AddUser adds a VLESS user to an inbound identified by its tag.
func (c *Client) AddUser(ctx context.Context, inboundTag string, userUUID uuid.UUID, email string) error {
	_, err := c.proxyman.AlterInbound(ctx, &proxymanCommand.AlterInboundRequest{
		Tag: inboundTag,
		Operation: serial.ToTypedMessage(&proxymanCommand.AddUserOperation{
			User: &protocol.User{
				Email: email,
				Account: serial.ToTypedMessage(&vless.Account{
					Id: userUUID.String(),
				}),
			},
		}),
	})
	if err != nil {
		return fmt.Errorf("xray add user %s to %s: %w", userUUID, inboundTag, err)
	}
	return nil
}

// RemoveUser removes a user from an inbound by email.
func (c *Client) RemoveUser(ctx context.Context, inboundTag string, email string) error {
	_, err := c.proxyman.AlterInbound(ctx, &proxymanCommand.AlterInboundRequest{
		Tag: inboundTag,
		Operation: serial.ToTypedMessage(&proxymanCommand.RemoveUserOperation{
			Email: email,
		}),
	})
	if err != nil {
		return fmt.Errorf("xray remove user %s from %s: %w", email, inboundTag, err)
	}
	return nil
}

// UserTraffic holds up/down bytes for a user.
type UserTraffic struct {
	Email     string
	BytesUp   int64
	BytesDown int64
}

// GetUserTraffic queries traffic stats for a user and optionally resets the counter.
func (c *Client) GetUserTraffic(ctx context.Context, email string, reset bool) (*UserTraffic, error) {
	upName := fmt.Sprintf("user>>>%s>>>traffic>>>uplink", email)
	downName := fmt.Sprintf("user>>>%s>>>traffic>>>downlink", email)

	up, err := c.queryStat(ctx, upName, reset)
	if err != nil {
		return nil, err
	}
	down, err := c.queryStat(ctx, downName, reset)
	if err != nil {
		return nil, err
	}

	return &UserTraffic{
		Email:     email,
		BytesUp:   up,
		BytesDown: down,
	}, nil
}

func (c *Client) queryStat(ctx context.Context, name string, reset bool) (int64, error) {
	resp, err := c.stats.GetStats(ctx, &statsCommand.GetStatsRequest{
		Name:   name,
		Reset_: reset,
	})
	if err != nil {
		// stat not found means no traffic yet
		return 0, nil
	}
	return resp.Stat.Value, nil
}

// UserEmail returns the canonical email used in xray stats for a subscription.
// Format: sub_{token} — keeps it short and unique.
func UserEmail(subscriptionToken string) string {
	return "sub_" + subscriptionToken
}
