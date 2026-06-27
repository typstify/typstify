package utils

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"

	bolt "go.etcd.io/bbolt"
)

var (
	KeyNotFoundError = errors.New("key not found")
)

type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~float32 | ~float64 |
		~[]int | ~[]int8 | ~[]int16 | ~[]int32 | ~[]int64 | ~[]uint | ~[]uint8 | ~[]uint16 | ~[]uint32 | ~[]uint64 | ~[]float32 | []float64
}

type Entity interface {
	any
}

type Key interface {
	Bytes() []byte
}

type ValEncoder[T any] interface {
	Encode(v *T) ([]byte, error)
	Decode(data []byte, t *T) error
}

// String index key for Bucket
type SKey string

// int index key for Bucket
type IKey uint64

func (k SKey) Bytes() []byte {
	return []byte(k)
}

func IntKey(k string) IKey {
	return IKey(hashStringToInt(string(k)))
}

// hashStringToInt hashes a string to a uint64 using SHA-256
func hashStringToInt(s string) uint64 {
	h := sha256.New()
	h.Write([]byte(s))
	hashBytes := h.Sum(nil)
	return binary.BigEndian.Uint64(hashBytes[:8]) // Use the first 8 bytes of the hash
}

func (k IKey) Bytes() []byte {
	buf := make([]byte, 8) // uint64 is 8 bytes
	binary.BigEndian.PutUint64(buf, uint64(k))
	return buf
}

type JsonEncoder[T any] struct {
}

type StringEncoder struct{}

type BinaryEncoder[T any] struct {
	rlock sync.Mutex
	wlock sync.Mutex
}

func (enc *JsonEncoder[T]) Encode(v *T) ([]byte, error) {
	return json.Marshal(v)
}

func (enc *JsonEncoder[T]) Decode(data []byte, v *T) error {
	return json.Unmarshal(data, v)
}

func (enc *StringEncoder) Encode(v *string) ([]byte, error) {
	return []byte(*v), nil
}

func (enc *StringEncoder) Decode(data []byte, v *string) error {
	*v = string(data)
	return nil
}

func (enc *BinaryEncoder[T]) Encode(v *T) ([]byte, error) {
	enc.rlock.Lock()
	defer enc.rlock.Unlock()

	var buf bytes.Buffer
	gob.Register(v)
	encoder := gob.NewEncoder(&buf)

	err := encoder.Encode(v)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (enc *BinaryEncoder[T]) Decode(data []byte, v *T) error {
	enc.wlock.Lock()
	defer enc.wlock.Unlock()

	gob.Register(v)
	decoder := gob.NewDecoder(bytes.NewReader(data))

	return decoder.Decode(v)
}

type Bucket[K Key, T Entity] struct {
	name       string
	db         *bolt.DB
	valEncoder ValEncoder[T]
}

func NewBucket[K Key, T Entity](name string, db *bolt.DB, valEncoder ValEncoder[T]) *Bucket[K, T] {
	err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(name))
		if err != nil {
			return fmt.Errorf("create bucket failed: %s", err)
		}
		return nil
	})
	if err != nil {
		log.Println("create bucket error: ", err)
		return nil
	}

	return &Bucket[K, T]{
		name:       name,
		db:         db,
		valEncoder: valEncoder,
	}
}

func (b *Bucket[K, T]) Save(key K, val T) error {
	// json.NewEncoder()
	v, err := b.valEncoder.Encode(&val)
	if err != nil {
		return err
	}
	err = b.write(key, v)
	return err
}

func (b *Bucket[K, T]) SaveAll(keys []K, vals []T) error {
	valBytes := make([][]byte, len(vals))
	for i, val := range vals {
		v, err := b.valEncoder.Encode(&val)
		if err != nil {
			return err
		}
		valBytes[i] = v
	}

	err := b.multiWrite(keys, valBytes)
	return err
}

func (b *Bucket[K, T]) Get(ID K) (T, error) {
	// json.NewEncoder()
	buf, err := b.read(ID)
	if err != nil {
		return *new(T), err
	}

	var entity T
	err = b.valEncoder.Decode(buf, &entity)
	if err != nil {
		return *new(T), err
	}

	return entity, nil
}

func (b *Bucket[K, T]) write(key K, p []byte) error {
	err := b.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(b.name))
		if bkt == nil {
			return fmt.Errorf("bucket not found: %s", b.name)
		}
		return bkt.Put(key.Bytes(), p)
	})

	if err != nil {
		return err
	}

	return nil
}

func (b *Bucket[K, T]) multiWrite(keys []K, values [][]byte) error {
	err := b.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(b.name))
		if bkt == nil {
			return fmt.Errorf("bucket not found: %s", b.name)
		}
		for i, k := range keys {
			err := bkt.Put(k.Bytes(), values[i])
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

func (b *Bucket[K, T]) read(key K) ([]byte, error) {
	var buf []byte
	err := b.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(b.name))
		if bkt == nil {
			return fmt.Errorf("bucket not found: %s", b.name)
		}
		buf = bkt.Get(key.Bytes())
		if buf == nil {
			return KeyNotFoundError
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return buf, nil
}

func (b *Bucket[K, T]) GetAll() ([]T, error) {
	var results []T

	err := b.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(b.name))
		if bkt == nil {
			return fmt.Errorf("bucket not found: %s", b.name)
		}

		cur := bkt.Cursor()
		for k, v := cur.First(); k != nil; k, v = cur.Next() {
			var entity T
			err := b.valEncoder.Decode(v, &entity)
			if err != nil {
				return err
			}
			results = append(results, entity)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return results, nil
}

func (b *Bucket[K, T]) GetByPrefix(prefix string) ([]T, error) {
	var results []T

	err := b.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(b.name))
		if bkt == nil {
			return fmt.Errorf("bucket not found: %s", b.name)
		}

		cur := bkt.Cursor()
		pf := []byte(prefix)
		for k, v := cur.Seek(pf); k != nil && bytes.HasPrefix(k, pf); k, v = cur.Next() {
			var entity T
			err := b.valEncoder.Decode(v, &entity)
			if err != nil {
				return err
			}
			results = append(results, entity)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return results, nil
}

func (b *Bucket[K, T]) Delete(key K) error {
	err := b.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(b.name))
		if bkt == nil {
			return fmt.Errorf("bucket not found: %s", b.name)
		}
		return bkt.Delete(key.Bytes())
	})

	if err != nil {
		return err
	}

	return nil
}

func (b *Bucket[K, T]) DeleteByPrefix(prefix string) error {
	return b.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(b.name))
		if bkt == nil {
			return fmt.Errorf("bucket not found: %s", b.name)
		}

		cur := bkt.Cursor()
		pf := []byte(prefix)
		for k, _ := cur.Seek(pf); k != nil && bytes.HasPrefix(k, pf); k, _ = cur.Next() {
			cur.Delete()
		}

		return nil
	})
}
