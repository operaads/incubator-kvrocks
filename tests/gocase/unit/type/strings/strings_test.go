/*
* Licensed to the Apache Software Foundation (ASF) under one
* or more contributor license agreements.  See the NOTICE file
* distributed with this work for additional information
* regarding copyright ownership.  The ASF licenses this file
* to you under the Apache License, Version 2.0 (the
* "License"); you may not use this file except in compliance
* with the License.  You may obtain a copy of the License at
*
*   http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing,
* software distributed under the License is distributed on an
* "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
* KIND, either express or implied.  See the License for the
* specific language governing permissions and limitations
* under the License.
 */

package strings

import (
	"context"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/apache/incubator-kvrocks/tests/gocase/util"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
)

func TestString(t *testing.T) {
	srv := util.StartServer(t, map[string]string{})
	defer srv.Close()
	ctx := context.Background()
	rdb := srv.NewClient()
	defer func() { require.NoError(t, rdb.Close()) }()

	t.Run("SET and GET an item", func(t *testing.T) {
		key := "x"
		value := "foobar"
		require.NoError(t, rdb.Set(ctx, key, value, 0).Err())
		require.Equal(t, value, rdb.Get(ctx, key).Val())
	})

	t.Run("SET and GET an empty item", func(t *testing.T) {
		key := "x"
		value := ""
		require.NoError(t, rdb.Set(ctx, key, value, 0).Err())
		require.Equal(t, value, rdb.Get(ctx, key).Val())
	})

	t.Run("Very big payload in GET/SET", func(t *testing.T) {
		key := "x"
		value := strings.Repeat("abcd", 1000000)
		require.NoError(t, rdb.Set(ctx, key, value, 0).Err())
		require.Equal(t, value, rdb.Get(ctx, key).Val())
	})

	t.Run("Very big payload random access", func(t *testing.T) {
		var payload []string
		for i := 0; i < 100; i++ {
			buf := util.RandString(1, 100000, util.Alpha)
			payload = append(payload, buf)
			require.NoError(t, rdb.Set(ctx, "bigpayload_"+strconv.Itoa(i), buf, 0).Err())
		}

		for i := 0; i < 1000; i++ {
			index := util.RandomInt(100)
			key := "bigpayload_" + strconv.FormatInt(index, 10)
			buf := rdb.Get(ctx, key).Val()
			require.Equal(t, payload[index], buf)
		}
	})

	t.Run("SET 10000 numeric keys and access all them in reverse order", func(t *testing.T) {
		require.NoError(t, rdb.FlushDB(ctx).Err())
		for i := 0; i < 10000; i++ {
			key := strconv.Itoa(i)
			value := key
			require.NoError(t, rdb.Set(ctx, key, value, 0).Err())
		}

		for i := 9999; i >= 0; i-- {
			key := strconv.Itoa(i)
			value := key
			require.EqualValues(t, value, rdb.Get(ctx, key).Val())
		}
	})

	t.Run("SETNX target key missing", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "novar").Err())
		require.True(t, rdb.SetNX(ctx, "novar", "foobared", 0).Val())
		require.Equal(t, "foobared", rdb.Get(ctx, "novar").Val())
	})

	t.Run("SETNX target key exists", func(t *testing.T) {
		require.NoError(t, rdb.Set(ctx, "novar", "foobared", 0).Err())
		require.False(t, rdb.SetNX(ctx, "novar", "blabla", 0).Val())
		require.Equal(t, "foobared", rdb.Get(ctx, "novar").Val())
	})

	t.Run("SETNX against not-expired volatile key", func(t *testing.T) {
		require.NoError(t, rdb.Set(ctx, "x", "10", 0).Err())
		require.NoError(t, rdb.Expire(ctx, "x", 10000*time.Second).Err())
		require.False(t, rdb.SetNX(ctx, "x", "20", 0).Val())
		require.Equal(t, "10", rdb.Get(ctx, "x").Val())
	})

	t.Run("SETNX against expired volatile key", func(t *testing.T) {
		// Make it very unlikely for the key this test uses to be expired by the
		// active expiry cycle. This is tightly coupled to the implementation of
		// active expiry and dbAdd() but currently the only way to test that
		// SETNX expires a key when it should have been.
		for x := 0; x < 9999; x++ {
			require.NoError(t, rdb.SetEx(ctx, "key-"+"x", "value", 3600*time.Second).Err())
		}

		// This will be one of 10000 expiring keys. A cycle is executed every
		// 100ms, sampling 10 keys for being expired or not.  This key will be
		// expired for at most 1s when we wait 2s, resulting in a total sample
		// of 100 keys. The probability of the success of this test being a
		// false positive is therefore approx. 1%.
		require.NoError(t, rdb.Set(ctx, "x", "10", 0).Err())
		require.NoError(t, rdb.Expire(ctx, "x", time.Second).Err())

		// Wait for the key to expire
		time.Sleep(2000 * time.Millisecond)

		require.NoError(t, rdb.SetNX(ctx, "x", "20", 0).Err())
		require.Equal(t, "20", rdb.Get(ctx, "x").Val())
	})

	t.Run("GETEX EX option", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "foo").Err())
		require.NoError(t, rdb.Set(ctx, "foo", "bar", 0).Err())
		require.NoError(t, rdb.GetEx(ctx, "foo", 10*time.Second).Err())
		util.BetweenValues(t, rdb.TTL(ctx, "foo").Val(), 5*time.Second, 10*time.Second)
	})

	t.Run("GETEX PX option", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "foo").Err())
		require.NoError(t, rdb.Set(ctx, "foo", "bar", 0).Err())
		require.NoError(t, rdb.GetEx(ctx, "foo", 10000*time.Millisecond).Err())
		util.BetweenValues(t, rdb.TTL(ctx, "foo").Val(), 5000*time.Millisecond, 10000*time.Millisecond)
	})

	t.Run("GETEX EXAT option", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "foo").Err())
		require.NoError(t, rdb.Set(ctx, "foo", "bar", 0).Err())
		require.NoError(t, rdb.Do(ctx, "getex", "foo", "exat", time.Now().Add(10*time.Second).Unix()).Err())
		util.BetweenValues(t, rdb.TTL(ctx, "foo").Val(), 5*time.Second, 10*time.Second)
	})

	t.Run("GETEX PXAT option", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "foo").Err())
		require.NoError(t, rdb.Set(ctx, "foo", "bar", 0).Err())
		require.NoError(t, rdb.Do(ctx, "getex", "foo", "pxat", time.Now().Add(10000*time.Millisecond).UnixMilli()).Err())
		util.BetweenValues(t, rdb.TTL(ctx, "foo").Val(), 5000*time.Millisecond, 10000*time.Millisecond)
	})

	t.Run("GETEX PERSIST option", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "foo").Err())
		require.NoError(t, rdb.Set(ctx, "foo", "bar", 10*time.Second).Err())
		util.BetweenValues(t, rdb.TTL(ctx, "foo").Val(), 5*time.Second, 10*time.Second)
		require.NoError(t, rdb.Do(ctx, "getex", "foo", "persist").Err())
		require.EqualValues(t, -1, rdb.TTL(ctx, "foo").Val())
	})

	t.Run("GETEX no option", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "foo").Err())
		require.NoError(t, rdb.Set(ctx, "foo", "bar", 0).Err())
		require.Equal(t, "bar", rdb.GetEx(ctx, "foo", 0).Val())
	})

	t.Run("GETEX syntax errors", func(t *testing.T) {
		util.ErrorRegexp(t, rdb.Do(ctx, "getex", "foo", "non-existent-option").Err(), ".*syntax*.")
	})

	t.Run("GETEX no arguments", func(t *testing.T) {
		util.ErrorRegexp(t, rdb.Do(ctx, "getex").Err(), ".*wrong number of arguments*.")
	})

	t.Run("GETDEL command", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "foo").Err())
		require.NoError(t, rdb.Set(ctx, "foo", "bar", 0).Err())
		require.Equal(t, "bar", rdb.GetDel(ctx, "foo").Val())
		require.Equal(t, "", rdb.GetDel(ctx, "foo").Val())
	})

	t.Run("MGET command", func(t *testing.T) {
		require.NoError(t, rdb.FlushDB(ctx).Err())
		require.NoError(t, rdb.Set(ctx, "foo", "BAR", 0).Err())
		require.NoError(t, rdb.Set(ctx, "bar", "FOO", 0).Err())
		require.Equal(t, []interface{}{"BAR", "FOO"}, rdb.MGet(ctx, "foo", "bar").Val())
	})

	t.Run("MGET against non existing key", func(t *testing.T) {
		require.Equal(t, []interface{}{"BAR", nil, "FOO"}, rdb.MGet(ctx, "foo", "baazz", "bar").Val())
	})

	t.Run("MGET against non-string key", func(t *testing.T) {
		require.NoError(t, rdb.SAdd(ctx, "myset", "ciao", "bau").Err())
		require.Equal(t, []interface{}{"BAR", nil, "FOO", nil}, rdb.MGet(ctx, "foo", "baazz", "bar", "myset").Val())
	})

	t.Run("GETSET set new value", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "foo").Err())
		require.Equal(t, "", rdb.GetSet(ctx, "foo", "xyz").Val())
		require.Equal(t, "xyz", rdb.Get(ctx, "foo").Val())
	})

	t.Run("GETSET replace old value", func(t *testing.T) {
		require.NoError(t, rdb.Set(ctx, "foo", "bar", 0).Err())
		require.Equal(t, "bar", rdb.GetSet(ctx, "foo", "xyz").Val())
		require.Equal(t, "xyz", rdb.Get(ctx, "foo").Val())
	})

	t.Run("MSET base case", func(t *testing.T) {
		require.NoError(t, rdb.MSet(ctx, map[string]interface{}{
			"x": "10",
			"y": "foo bar",
			"z": "x x x x x x x\n\n\r\n",
		}).Err())
		require.Equal(t, []interface{}{"10", "foo bar", "x x x x x x x\n\n\r\n"}, rdb.MGet(ctx, "x", "y", "z").Val())
	})

	t.Run("MSET wrong number of args", func(t *testing.T) {
		r := rdb.MSet(ctx, "x", "10", "y", "foo bar", "z")
		require.ErrorContains(t, r.Err(), "wrong number")
	})

	t.Run("MSETNX with already existent key", func(t *testing.T) {
		r := rdb.MSetNX(ctx, map[string]interface{}{
			"x1": "xxx",
			"y2": "yyy",
			"x":  "20",
		})
		require.False(t, r.Val())
		require.EqualValues(t, 0, rdb.Exists(ctx, "x1").Val())
		require.EqualValues(t, 0, rdb.Exists(ctx, "y2").Val())
	})

	t.Run("MSETNX with not existing keys", func(t *testing.T) {
		r := rdb.MSetNX(ctx, map[string]interface{}{
			"x1": "xxx",
			"y2": "yyy",
		})
		require.True(t, r.Val())
		require.Equal(t, "xxx", rdb.Get(ctx, "x1").Val())
		require.Equal(t, "yyy", rdb.Get(ctx, "y2").Val())
	})

	t.Run("STRLEN against non-existing key", func(t *testing.T) {
		require.EqualValues(t, 0, rdb.StrLen(ctx, "notakey").Val())
	})

	t.Run("STRLEN against integer-encoded value", func(t *testing.T) {
		require.NoError(t, rdb.Set(ctx, "myinteger", -555, 0).Err())
		require.EqualValues(t, 4, rdb.StrLen(ctx, "myinteger").Val())
	})

	t.Run("STRLEN against plain string", func(t *testing.T) {
		require.NoError(t, rdb.Set(ctx, "mystring", "foozzz0123456789 baz", 0).Err())
		require.EqualValues(t, 20, rdb.StrLen(ctx, "mystring").Val())
	})

	t.Run("SETBIT against key with wrong type", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "mykey").Err())
		require.NoError(t, rdb.LPush(ctx, "mykey", "foo").Err())
		require.ErrorContains(t, rdb.SetBit(ctx, "mykey", 0, 1).Err(), "WRONGTYPE")
	})

	t.Run("SETBIT with out of range bit offset", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "mykey").Err())
		require.ErrorContains(t, rdb.SetBit(ctx, "mykey", 4*1024*1024*1024+2, 1).Err(), "out of range")
		require.ErrorContains(t, rdb.SetBit(ctx, "mykey", -1, 1).Err(), "out of range")
	})

	t.Run("SETBIT with non-bit argument", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "mykey").Err())
		require.ErrorContains(t, rdb.SetBit(ctx, "mykey", 1, -1).Err(), "out of range")
		require.ErrorContains(t, rdb.SetBit(ctx, "mykey", 1, 2).Err(), "out of range")
		require.ErrorContains(t, rdb.SetBit(ctx, "mykey", 1, 10).Err(), "out of range")
		require.ErrorContains(t, rdb.SetBit(ctx, "mykey", 1, 20).Err(), "out of range")
	})

	t.Run("SETBIT/GETBIT/BITCOUNT/BITPOS boundary check (type string)", func(t *testing.T) {
		// When setting the last possible bit (offset equal to 2^32 -1) and the string value stored at key
		// does not yet hold a string value, or holds a small string value, Kvrocks needs to allocate all
		// intermediate memory which can block the server for some time. See also https://redis.io/commands/setbit/.
		rdb := srv.NewClientWithOption(&redis.Options{
			ReadTimeout: time.Minute,
		})
		defer func() { require.NoError(t, rdb.Close()) }()

		require.NoError(t, rdb.Del(ctx, "mykey").Err())
		require.NoError(t, rdb.Set(ctx, "mykey", "", 0).Err())
		var maxOffset int64 = math.MaxInt32
		require.NoError(t, rdb.SetBit(ctx, "mykey", maxOffset, 1).Err())
		require.EqualValues(t, 1, rdb.GetBit(ctx, "mykey", maxOffset).Val())
		require.EqualValues(t, 1, rdb.BitCount(ctx, "mykey", &redis.BitCount{Start: 0, End: maxOffset / 8}).Val())
		require.EqualValues(t, maxOffset, rdb.BitPos(ctx, "mykey", 1).Val())
	})

	t.Run("GETBIT against string-encoded key", func(t *testing.T) {
		// Single byte with 2nd and 3rd bit set
		require.NoError(t, rdb.Set(ctx, "mykey", "`", 0).Err())

		// In-range
		require.EqualValues(t, 0, rdb.GetBit(ctx, "mykey", 0).Val())
		require.EqualValues(t, 1, rdb.GetBit(ctx, "mykey", 1).Val())
		require.EqualValues(t, 1, rdb.GetBit(ctx, "mykey", 2).Val())
		require.EqualValues(t, 0, rdb.GetBit(ctx, "mykey", 3).Val())

		// Out-range
		require.EqualValues(t, 0, rdb.GetBit(ctx, "mykey", 8).Val())
		require.EqualValues(t, 0, rdb.GetBit(ctx, "mykey", 100).Val())
		require.EqualValues(t, 0, rdb.GetBit(ctx, "mykey", 10000).Val())
	})

	t.Run("GETBIT against integer-encoded key", func(t *testing.T) {
		require.NoError(t, rdb.Set(ctx, "mykey", 1, 0).Err())

		// Ascii "1" is integer 49 = 00 11 00 01
		require.EqualValues(t, 0, rdb.GetBit(ctx, "mykey", 0).Val())
		require.EqualValues(t, 0, rdb.GetBit(ctx, "mykey", 1).Val())
		require.EqualValues(t, 1, rdb.GetBit(ctx, "mykey", 2).Val())
		require.EqualValues(t, 1, rdb.GetBit(ctx, "mykey", 3).Val())

		// Out-range
		require.EqualValues(t, 0, rdb.GetBit(ctx, "mykey", 8).Val())
		require.EqualValues(t, 0, rdb.GetBit(ctx, "mykey", 100).Val())
		require.EqualValues(t, 0, rdb.GetBit(ctx, "mykey", 10000).Val())
	})

	t.Run("SETRANGE against non-existing key", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "mykey").Err())
		require.EqualValues(t, 3, rdb.SetRange(ctx, "mykey", 0, "foo").Val())
		require.Equal(t, "foo", rdb.Get(ctx, "mykey").Val())

		require.NoError(t, rdb.Del(ctx, "mykey").Err())
		require.EqualValues(t, 0, rdb.SetRange(ctx, "mykey", 0, "").Val())
		require.EqualValues(t, 0, rdb.Exists(ctx, "mykey").Val())

		require.NoError(t, rdb.Del(ctx, "mykey").Err())
		require.EqualValues(t, 4, rdb.SetRange(ctx, "mykey", 1, "foo").Val())
		require.Equal(t, "\000foo", rdb.Get(ctx, "mykey").Val())
	})

	t.Run("SETRANGE against string-encoded key", func(t *testing.T) {
		require.NoError(t, rdb.Set(ctx, "mykey", "foo", 0).Err())
		require.EqualValues(t, 3, rdb.SetRange(ctx, "mykey", 0, "b").Val())
		require.Equal(t, "boo", rdb.Get(ctx, "mykey").Val())

		require.NoError(t, rdb.Set(ctx, "mykey", "foo", 0).Err())
		require.EqualValues(t, 3, rdb.SetRange(ctx, "mykey", 0, "").Val())
		require.Equal(t, "foo", rdb.Get(ctx, "mykey").Val())

		require.NoError(t, rdb.Set(ctx, "mykey", "foo", 0).Err())
		require.EqualValues(t, 3, rdb.SetRange(ctx, "mykey", 1, "b").Val())
		require.Equal(t, "fbo", rdb.Get(ctx, "mykey").Val())

		require.NoError(t, rdb.Set(ctx, "mykey", "foo", 0).Err())
		require.EqualValues(t, 7, rdb.SetRange(ctx, "mykey", 4, "bar").Val())
		require.Equal(t, "foo\000bar", rdb.Get(ctx, "mykey").Val())
	})

	t.Run("SETRANGE against integer-encoded key", func(t *testing.T) {
		require.NoError(t, rdb.Set(ctx, "mykey", 1234, 0).Err())
		require.EqualValues(t, 4, rdb.SetRange(ctx, "mykey", 0, "2").Val())
		require.Equal(t, "2234", rdb.Get(ctx, "mykey").Val())

		require.NoError(t, rdb.Set(ctx, "mykey", 1234, 0).Err())
		require.EqualValues(t, 4, rdb.SetRange(ctx, "mykey", 0, "").Val())
		require.Equal(t, "1234", rdb.Get(ctx, "mykey").Val())

		require.NoError(t, rdb.Set(ctx, "mykey", 1234, 0).Err())
		require.EqualValues(t, 4, rdb.SetRange(ctx, "mykey", 1, "3").Val())
		require.Equal(t, "1334", rdb.Get(ctx, "mykey").Val())

		require.NoError(t, rdb.Set(ctx, "mykey", 1234, 0).Err())
		require.EqualValues(t, 6, rdb.SetRange(ctx, "mykey", 5, "2").Val())
		require.Equal(t, "1234\0002", rdb.Get(ctx, "mykey").Val())
	})

	t.Run("SETRANGE against key with wrong type", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "mykey").Err())
		require.NoError(t, rdb.LPush(ctx, "mykey", "foo").Err())
		require.ErrorContains(t, rdb.SetRange(ctx, "mykey", 0, "bar").Err(), "WRONGTYPE")
	})

	t.Run("GETRANGE against non-existing key", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "mykey").Err())
		require.EqualValues(t, "", rdb.GetRange(ctx, "mykey", 0, -1).Val())
	})

	t.Run("GETRANGE against string value", func(t *testing.T) {
		require.NoError(t, rdb.Set(ctx, "mykey", "Hello World", 0).Err())
		require.EqualValues(t, "Hell", rdb.GetRange(ctx, "mykey", 0, 3).Val())
		require.EqualValues(t, "ll", rdb.GetRange(ctx, "mykey", 2, 3).Val())
		require.EqualValues(t, "Hello World", rdb.GetRange(ctx, "mykey", 0, -1).Val())
		require.EqualValues(t, "orld", rdb.GetRange(ctx, "mykey", -4, -1).Val())
		require.EqualValues(t, "", rdb.GetRange(ctx, "mykey", 5, 3).Val())
		require.EqualValues(t, " World", rdb.GetRange(ctx, "mykey", 5, 5000).Val())
		require.EqualValues(t, "Hello World", rdb.GetRange(ctx, "mykey", -5000, 10000).Val())
	})

	t.Run("GETRANGE against integer-encoded value", func(t *testing.T) {
		require.NoError(t, rdb.Set(ctx, "mykey", 1234, 0).Err())
		require.EqualValues(t, "123", rdb.GetRange(ctx, "mykey", 0, 2).Val())
		require.EqualValues(t, "1234", rdb.GetRange(ctx, "mykey", 0, -1).Val())
		require.EqualValues(t, "234", rdb.GetRange(ctx, "mykey", -3, -1).Val())
		require.EqualValues(t, "", rdb.GetRange(ctx, "mykey", 5, 3).Val())
		require.EqualValues(t, "4", rdb.GetRange(ctx, "mykey", 3, 5000).Val())
		require.EqualValues(t, "1234", rdb.GetRange(ctx, "mykey", -5000, 10000).Val())
	})

	t.Run("Extended SET can detect syntax errors", func(t *testing.T) {
		require.ErrorContains(t, rdb.Do(ctx, "SET", "foo", "bar", "non-existing-option").Err(), "syntax error")
	})

	t.Run("Extended SET NX option", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "foo").Err())
		require.Equal(t, "OK", rdb.Do(ctx, "SET", "foo", "1", "nx").Val())
		require.Nil(t, rdb.Do(ctx, "SET", "foo", "2", "nx").Val())
		require.Equal(t, "1", rdb.Get(ctx, "foo").Val())
	})

	t.Run("Extended SET XX option", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "foo").Err())
		require.Nil(t, rdb.Do(ctx, "SET", "foo", "1", "xx").Val())
		require.NoError(t, rdb.Set(ctx, "foo", "bar", 0).Err())
		require.Equal(t, "OK", rdb.Do(ctx, "SET", "foo", "2", "xx").Val())
		require.Equal(t, "2", rdb.Get(ctx, "foo").Val())
	})

	t.Run("Extended SET EX option", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "foo").Err())
		require.Equal(t, "OK", rdb.Do(ctx, "SET", "foo", "bar", "ex", "10").Val())
		ttl := rdb.TTL(ctx, "foo").Val()
		util.BetweenValues(t, ttl, 5*time.Second, 10*time.Second)
	})

	t.Run("Extended SET PX option", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "foo").Err())
		require.Equal(t, "OK", rdb.Do(ctx, "SET", "foo", "bar", "px", "10000").Val())
		ttl := rdb.TTL(ctx, "foo").Val()
		util.BetweenValues(t, ttl, 5*time.Second, 10*time.Second)
	})

	t.Run("Extended SET EXAT option", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "foo").Err())

		expireAt := strconv.FormatInt(time.Now().Add(10*time.Second).Unix(), 10)
		require.Equal(t, "OK", rdb.Do(ctx, "SET", "foo", "bar", "exat", expireAt).Val())
		ttl := rdb.TTL(ctx, "foo").Val()
		util.BetweenValues(t, ttl, 5*time.Second, 10*time.Second)
	})

	t.Run("Extended SET EXAT option with expired timestamp", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "foo").Err())
		require.Equal(t, "OK", rdb.Do(ctx, "SET", "foo", "bar", "exat", "1").Val())
		require.Equal(t, "", rdb.Get(ctx, "foo").Val())

		require.NoError(t, rdb.Set(ctx, "foo", "bar", 0).Err())
		require.Equal(t, "bar", rdb.Get(ctx, "foo").Val())

		expireAt := strconv.FormatInt(time.Now().Add(-5*time.Second).Unix(), 10)
		require.Equal(t, "OK", rdb.Do(ctx, "SET", "foo", "bar", "exat", expireAt).Val())
		require.Equal(t, "", rdb.Get(ctx, "foo").Val())
	})

	t.Run("Extended SET PXAT option", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "foo").Err())

		expireAt := strconv.FormatInt(time.Now().Add(10*time.Second).UnixMilli(), 10)
		require.Equal(t, "OK", rdb.Do(ctx, "SET", "foo", "bar", "pxat", expireAt).Val())

		ttl := rdb.TTL(ctx, "foo").Val()
		util.BetweenValues(t, ttl, 5*time.Second, 10*time.Second)
	})

	t.Run("Extended SET PXAT option with expired timestamp", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "foo").Err())
		require.Equal(t, "OK", rdb.Do(ctx, "SET", "foo", "bar", "pxat", "1").Val())
		require.Equal(t, "", rdb.Get(ctx, "foo").Val())

		require.NoError(t, rdb.Set(ctx, "foo", "bar", 0).Err())

		expireAt := strconv.FormatInt(time.Now().Add(-5*time.Second).UnixMilli(), 10)
		require.Equal(t, "OK", rdb.Do(ctx, "SET", "foo", "bar", "pxat", expireAt).Val())
		require.Equal(t, "", rdb.Get(ctx, "foo").Val())
	})

	t.Run("Extended SET with incorrect use of multi options should result in syntax err", func(t *testing.T) {
		require.ErrorContains(t, rdb.Do(ctx, "SET", "foo", "bar", "ex", "10", "px", "10000").Err(), "syntax err")
		require.ErrorContains(t, rdb.Do(ctx, "SET", "foo", "bar", "NX", "XX").Err(), "syntax err")
	})

	t.Run("Extended SET with incorrect expire value", func(t *testing.T) {
		require.ErrorContains(t, rdb.Do(ctx, "SET", "foo", "bar", "ex", "1234xyz").Err(), "not an integer")
		require.ErrorContains(t, rdb.Do(ctx, "SET", "foo", "bar", "ex", "0").Err(), "invalid expire time")
		require.ErrorContains(t, rdb.Do(ctx, "SET", "foo", "bar", "exat", "1234xyz").Err(), "not an integer")
		require.ErrorContains(t, rdb.Do(ctx, "SET", "foo", "bar", "exat", "0").Err(), "invalid expire time")
		require.ErrorContains(t, rdb.Do(ctx, "SET", "foo", "bar", "pxat", "1234xyz").Err(), "not an integer")
		require.ErrorContains(t, rdb.Do(ctx, "SET", "foo", "bar", "pxat", "0").Err(), "invalid expire time")
	})

	t.Run("Extended SET using multiple options at once", func(t *testing.T) {
		require.NoError(t, rdb.Set(ctx, "foo", "bar", 0).Err())

		require.Equal(t, "OK", rdb.Do(ctx, "SET", "foo", "bar", "xx", "px", "10000").Val())
		ttl := rdb.TTL(ctx, "foo").Val()
		util.BetweenValues(t, ttl, 5*time.Second, 10*time.Second)
	})

	t.Run("GETRANGE with huge ranges, Github issue #1844", func(t *testing.T) {
		require.NoError(t, rdb.Set(ctx, "foo", "bar", 0).Err())
		require.Equal(t, "bar", rdb.GetRange(ctx, "foo", 0, 2094967291).Val())
	})

	t.Run("CAS normal case", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "cas_key").Err())

		require.EqualValues(t, -1, rdb.Do(ctx, "CAS", "cas_key", "old_value", "new_value").Val())
		require.EqualValues(t, 0, rdb.Exists(ctx, "cas_key").Val())
		require.Equal(t, "OK", rdb.Set(ctx, "cas_key", "old_value", 0).Val())
		require.EqualValues(t, 0, rdb.Do(ctx, "CAS", "cas_key", "old_val", "new_value").Val())
		require.EqualValues(t, 1, rdb.Do(ctx, "CAS", "cas_key", "old_value", "new_value").Val())
	})

	t.Run("CAS wrong key type", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "a_list_key").Err())
		require.NoError(t, rdb.LPush(ctx, "a_list_key", "123").Err())
		require.ErrorContains(t, rdb.Do(ctx, "CAS", "a_list_key", "123", "234").Err(), "WRONGTYPE")
	})

	t.Run("CAS invalid param num", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "cas_key").Err())
		require.NoError(t, rdb.Set(ctx, "cas_key", "123", 0).Err())
		require.ErrorContains(t, rdb.Do(ctx, "CAS", "cas_key", "123").Err(), "ERR wrong number of arguments")
		require.ErrorContains(t, rdb.Do(ctx, "CAS", "cas_key", "123", "234", "ex").Err(), "ERR wrong number of arguments")
	})

	t.Run("CAS expire", func(t *testing.T) {
		require.NoError(t, rdb.Del(ctx, "cas_key").Err())
		require.NoError(t, rdb.Set(ctx, "cas_key", "123", 0).Err())

		require.EqualValues(t, 1, rdb.Do(ctx, "CAS", "cas_key", "123", "234", "ex", "1").Val())
		require.Equal(t, "234", rdb.Get(ctx, "cas_key").Val())

		time.Sleep(2000 * time.Millisecond)

		require.Equal(t, "", rdb.Get(ctx, "cas_key").Val())
	})

	t.Run("CAD normal case", func(t *testing.T) {
		require.EqualValues(t, -1, rdb.Do(ctx, "CAD", "cad_key", "123").Val())
		require.NoError(t, rdb.Set(ctx, "cad_key", "123", 0).Err())
		require.EqualValues(t, 0, rdb.Do(ctx, "CAD", "cad_key", "234").Val())
		require.EqualValues(t, 1, rdb.Do(ctx, "CAD", "cad_key", "123").Val())
		require.Equal(t, "", rdb.Get(ctx, "cad_key").Val())
	})

	t.Run("CAD invalid param num", func(t *testing.T) {
		require.NoError(t, rdb.Set(ctx, "cad_key", "123", 0).Err())

		require.ErrorContains(t, rdb.Do(ctx, "CAD", "cad_key").Err(), "ERR wrong number of arguments")
		require.ErrorContains(t, rdb.Do(ctx, "CAD", "cad_key", "123", "234").Err(), "ERR wrong number of arguments")
	})
}
