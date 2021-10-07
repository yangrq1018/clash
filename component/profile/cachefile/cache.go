package cachefile

import (
	"bytes"
	"encoding/gob"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/Dreamacro/clash/component/profile"
	C "github.com/Dreamacro/clash/constant"
	"github.com/Dreamacro/clash/log"

	bolt "go.etcd.io/bbolt"
)

var (
	initOnce     sync.Once
	fileMode     os.FileMode = 0666
	defaultCache *CacheFile

	bucketSelected = []byte("selected")
	bucketFakeip   = []byte("fakeip")
)

// CacheFile store and update the cache file
type CacheFile struct {
	DB *bolt.DB
}

func (c *CacheFile) SetSelected(group, selected string) {
	if !profile.StoreSelected.Load() {
		return
	} else if c.DB == nil {
		return
	}

	err := c.DB.Batch(func(t *bolt.Tx) error {
		bucket, err := t.CreateBucketIfNotExists(bucketSelected)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(group), []byte(selected))
	})

	if err != nil {
		log.Warnln("[CacheFile] write cache to %s failed: %s", c.DB.Path(), err.Error())
		return
	}
}

func (c *CacheFile) SelectedMap() map[string]string {
	if !profile.StoreSelected.Load() {
		return nil
	} else if c.DB == nil {
		return nil
	}

	mapping := map[string]string{}
	c.DB.View(func(t *bolt.Tx) error {
		bucket := t.Bucket(bucketSelected)
		if bucket == nil {
			return nil
		}

		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			mapping[string(k)] = string(v)
		}
		return nil
	})
	return mapping
}

func (c *CacheFile) PutFakeip(key, value []byte) error {
	if c.DB == nil {
		return nil
	}

	err := c.DB.Batch(func(t *bolt.Tx) error {
		bucket, err := t.CreateBucketIfNotExists(bucketFakeip)
		if err != nil {
			return err
		}
		return bucket.Put(key, value)
	})

	if err != nil {
		log.Warnln("[CacheFile] write cache to %s failed: %s", c.DB.Path(), err.Error())
	}

	return err
}

func (c *CacheFile) GetFakeip(key []byte) []byte {
	if c.DB == nil {
		return nil
	}

	tx, err := c.DB.Begin(false)
	if err != nil {
		return nil
	}
	defer tx.Rollback()

	bucket := tx.Bucket(bucketFakeip)
	if bucket == nil {
		return nil
	}

	return bucket.Get(key)
}

func (c *CacheFile) Close() error {
	return c.DB.Close()
}

// TODO: remove migrateCache until 2022
func migrateCache() {
	defer func() {
		db, err := bolt.Open(C.Path.Cache(), fileMode, &bolt.Options{Timeout: time.Second})
		if err != nil {
			log.Warnln("[CacheFile] can't open cache file: %s", err.Error())
		}
		defaultCache = &CacheFile{
			DB: db,
		}
	}()

	buf, err := ioutil.ReadFile(C.Path.OldCache())
	if err != nil {
		return
	}
	defer os.Remove(C.Path.OldCache())

	// read old cache file
	type cache struct {
		Selected map[string]string
	}
	model := &cache{
		Selected: map[string]string{},
	}
	bufReader := bytes.NewBuffer(buf)
	gob.NewDecoder(bufReader).Decode(model)

	// write to new cache file
	db, err := bolt.Open(C.Path.Cache(), fileMode, nil)
	if err != nil {
		return
	}
	defer db.Close()
	db.Batch(func(t *bolt.Tx) error {
		bucket, err := t.CreateBucketIfNotExists(bucketSelected)
		if err != nil {
			return err
		}
		for group, selected := range model.Selected {
			if err := bucket.Put([]byte(group), []byte(selected)); err != nil {
				return err
			}
		}
		return nil
	})
}

// Cache return singleton of CacheFile
func Cache() *CacheFile {
	initOnce.Do(migrateCache)

	return defaultCache
}
