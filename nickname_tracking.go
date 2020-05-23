package main

import (
	"sort"
	"time"

	"github.com/go-redis/redis"
)

// NewNicknameTracker creates a new redis connection that keeps track of user nicknames.
func NewNicknameTracker(redisAddress, redisPassword string, expiresIn time.Duration) (*NicknameTracker, error) {

	client := redis.NewClient(&redis.Options{
		Addr:     redisAddress,
		Password: redisPassword,
	})

	_, err := client.Ping().Result()
	if err != nil {
		return nil, err
	}

	return &NicknameTracker{client, expiresIn}, nil
}

// NicknameTracker defines a redis cache connection and an expiration delay for the inserted nicknames
type NicknameTracker struct {
	*redis.Client
	ExpirationDelay time.Duration
}

// Add a player to the nickname tracking
func (n *NicknameTracker) Add(p Player) error {
	if n == nil {
		return nil
	}

	tx := n.TxPipeline()

	tx.SAdd(string(p.IP), p.Name)
	tx.Expire(string(p.IP), n.ExpirationDelay)
	tx.SAdd(p.Name, string(p.IP))
	tx.Expire(p.Name, n.ExpirationDelay)

	_, err := tx.Exec()
	if err != nil {
		return err
	}

	return nil
}

// IPs returns a list of IPs a specific nickname has been seen with.
func (n *NicknameTracker) IPs(name string) ([]string, error) {
	if n == nil {
		return nil, nil
	}

	ips, err := n.SMembers(name).Result()

	if err != nil {
		return nil, err
	}

	sort.Sort(byName(ips))

	return ips, nil
}

// WhoIs return all associated nicknames with specific IPs
func (n *NicknameTracker) WhoIs(name string) ([]string, error) {
	if n == nil {
		return nil, nil
	}

	ips, err := n.SMembers(name).Result()

	if err != nil {
		return nil, err
	}

	nicknameMap := make(map[string]bool, len(ips))

	tx := n.TxPipeline()

	responses := make([]*redis.StringSliceCmd, 0, len(ips))

	for _, ip := range ips {
		responses = append(responses, tx.SMembers(ip))
	}

	_, err = tx.Exec()
	if err != nil {
		return nil, err
	}

	for _, resp := range responses {
		nicknames, err := resp.Result()
		if err != nil {
			continue
		}

		for _, nickname := range nicknames {
			nicknameMap[nickname] = true
		}

	}

	result := make([]string, len(nicknameMap))

	for nickname := range nicknameMap {
		result = append(result, nickname)
	}

	sort.Sort(byName(result))

	return result, nil
}
