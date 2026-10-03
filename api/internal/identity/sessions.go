package identity

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Sessions live in Redis: the cookie holds only a random ID. Deleting the
// key ends the session immediately, on every device for "sign out
// everywhere".
type sessions struct {
	rdb      *redis.Client
	idle     time.Duration // a session unused for this long ends
	lifetime time.Duration // a session ends this long after sign-in, used or not
}

var errNoSession = errors.New("identity: no session")

func sessionKey(id string) string         { return "session:" + id }
func userSessionsKey(id uuid.UUID) string { return "user-sessions:" + id.String() }

func (s *sessions) create(ctx context.Context, userID uuid.UUID) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(raw)
	pipe := s.rdb.TxPipeline()
	pipe.HSet(ctx, sessionKey(id), "user", userID.String(), "created", time.Now().Unix())
	pipe.Expire(ctx, sessionKey(id), s.idle)
	pipe.SAdd(ctx, userSessionsKey(userID), id)
	pipe.Expire(ctx, userSessionsKey(userID), s.lifetime)
	_, err := pipe.Exec(ctx)
	return id, err
}

// lookup returns the session's user and slides its idle expiry.
func (s *sessions) lookup(ctx context.Context, id string) (uuid.UUID, error) {
	values, err := s.rdb.HGetAll(ctx, sessionKey(id)).Result()
	if err != nil {
		return uuid.Nil, err
	}
	userID, err := uuid.Parse(values["user"])
	if err != nil {
		return uuid.Nil, errNoSession
	}
	created, _ := strconv.ParseInt(values["created"], 10, 64)
	if time.Since(time.Unix(created, 0)) > s.lifetime {
		s.delete(ctx, id, userID)
		return uuid.Nil, errNoSession
	}
	s.rdb.Expire(ctx, sessionKey(id), s.idle)
	return userID, nil
}

func (s *sessions) delete(ctx context.Context, id string, userID uuid.UUID) {
	s.rdb.Del(ctx, sessionKey(id))
	s.rdb.SRem(ctx, userSessionsKey(userID), id)
}

// deleteAll ends every session the user has.
func (s *sessions) deleteAll(ctx context.Context, userID uuid.UUID) error {
	ids, err := s.rdb.SMembers(ctx, userSessionsKey(userID)).Result()
	if err != nil {
		return err
	}
	keys := []string{userSessionsKey(userID)}
	for _, id := range ids {
		keys = append(keys, sessionKey(id))
	}
	return s.rdb.Del(ctx, keys...).Err()
}
