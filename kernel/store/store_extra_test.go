package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func TestOpenEmptyPath(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Fatal("Open(\"\") 应报错")
	}
}

// 写语句错误应经 writer 回传给调用方（不吞错、不让 store 卡死）。
func TestExecSQLErrorPropagates(t *testing.T) {
	s := openTemp(t)
	if _, err := s.Exec(context.Background(), `INSERT INTO no_such_table(x) VALUES(1)`); err == nil {
		t.Fatal("坏 SQL 应返回错误")
	}
	// store 仍可用。
	if _, err := s.Exec(context.Background(), `INSERT INTO app_meta(key,value) VALUES('ok','1')`); err != nil {
		t.Fatalf("store 应仍可用: %v", err)
	}
}

// Close 与并发 Exec 竞争：不得 panic；结果要么成功要么 ErrClosed。
func TestCloseDuringExecNoPanic(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "race.db"))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("goroutine %d panic: %v", i, r)
				}
			}()
			for j := 0; j < 200; j++ {
				_, err := s.Exec(context.Background(), `INSERT INTO app_meta(key,value) VALUES(?,?)`,
					fmt.Sprintf("k-%d-%d", i, j), "v")
				if err != nil && !errors.Is(err, ErrClosed) {
					t.Errorf("意外错误: %v", err)
					return
				}
			}
		}(i)
	}
	closeDone := make(chan struct{})
	go func() {
		_ = s.Close()
		close(closeDone)
	}()
	wg.Wait()
	<-closeDone
}
