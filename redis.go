package collector

import (
	"fmt"

	"github.com/redis/go-redis/v9"
)

func NewRedisClient(address, user, password string, db int) (*redis.Client, error) {
	opt, err := redis.ParseURL(fmt.Sprintf("rediss://%s:%s@%s", user, password, address))
	if err != nil {
		return nil, err
	}
	c := redis.NewClient(opt)
	return c, nil
}
