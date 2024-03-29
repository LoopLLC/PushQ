// Copyright 2011 Google Inc. All rights reserved.
// Use of this source code is governed by the Apache 2.0
// license that can be found in the LICENSE file.

// This file has a simple implemetation of datastore shard counters.

package pushq

import (
	"fmt"
	"math/rand"

	"golang.org/x/net/context"

	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/memcache"
)

type counterConfig struct {
	Shards int
}

type counterShard struct {
	Name  string
	Count int64
}

const (
	defaultShards = 20
	configKind    = "CounterConfig"
	shardKind     = "CounterShard"
)

func memcacheKey(name string) string {
	return shardKind + ":" + name
}

// Count retrieves the value of the named counter.
func Count(ctx context.Context, name string) (int64, error) {
	var total int64
	mkey := memcacheKey(name)
	if _, err := memcache.JSON.Get(ctx, mkey, &total); err == nil {
		return total, nil
	}
	q := datastore.NewQuery(shardKind).Filter("Name =", name)
	for t := q.Run(ctx); ; {
		var s counterShard
		_, err := t.Next(&s)
		if err == datastore.Done {
			break
		}
		if err != nil {
			return total, err
		}
		total += s.Count
	}
	memcache.JSON.Set(ctx, &memcache.Item{
		Key:        mkey,
		Object:     &total,
		Expiration: 60,
	})
	return total, nil
}

// Increment increments the named counter.
func Increment(ctx context.Context, name string, by int64) error {
	// Get counter config.
	var cfg counterConfig
	ckey := datastore.NewKey(ctx, configKind, name, 0, nil)
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		err := datastore.Get(ctx, ckey, &cfg)
		if err == datastore.ErrNoSuchEntity {
			cfg.Shards = defaultShards
			_, err = datastore.Put(ctx, ckey, &cfg)
		}
		return err
	}, nil)
	if err != nil {
		return err
	}
	var s counterShard
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		shardName := fmt.Sprintf("%s-shard%d", name, rand.Intn(cfg.Shards))
		key := datastore.NewKey(ctx, shardKind, shardName, 0, nil)
		err := datastore.Get(ctx, key, &s)
		// A missing entity and a present entity will both work.
		if err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}
		s.Name = name
		s.Count += by
		_, err = datastore.Put(ctx, key, &s)
		return err
	}, nil)
	if err != nil {
		return err
	}
	memcache.IncrementExisting(ctx, memcacheKey(name), 1)
	return nil
}

// IncreaseCounterShards increases the number of shards for the named
// counter to n. It will never decrease the number of shards.
func IncreaseCounterShards(ctx context.Context, name string, n int) error {
	ckey := datastore.NewKey(ctx, configKind, name, 0, nil)
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		var cfg counterConfig
		mod := false
		err := datastore.Get(ctx, ckey, &cfg)
		if err == datastore.ErrNoSuchEntity {
			cfg.Shards = defaultShards
			mod = true
		} else if err != nil {
			return err
		}
		if cfg.Shards < n {
			cfg.Shards = n
			mod = true
		}
		if mod {
			_, err = datastore.Put(ctx, ckey, &cfg)
		}
		return err
	}, nil)
}

// GetAllCounterNames returns an array of all counter names
func GetAllCounterNames(ctx context.Context) ([]string, error) {
	var retval []string
	q := datastore.NewQuery(configKind).KeysOnly()
	var configKeys []*datastore.Key
	var err error
	if configKeys, err = q.GetAll(ctx, nil); err != nil {
		return retval, err
	}
	for _, key := range configKeys {
		retval = append(retval, key.StringID())
	}

	return retval, nil
}
