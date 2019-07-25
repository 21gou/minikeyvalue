package main

import (
  "os"
  "fmt"
  "sync"
  "net/http"
  "strings"
  "github.com/syndtr/goleveldb/leveldb"
)

type RebalanceRequest struct {
  key []byte
  volumes []string
  kvolumes []string
}

func rebalance(db *leveldb.DB, req RebalanceRequest) bool {
  kp := key2path(req.key)

  // find the volumes that are real
  rvolumes := make([]string, 0)
  for _, rv := range req.volumes {
    if remote_head(fmt.Sprintf("http://%s%s", rv, kp)) {
      rvolumes = append(rvolumes, rv)
    }
  }

  if len(rvolumes) == 0 {
    fmt.Printf("can't rebalance, %s is missing!\n", string(req.key))
    return false
  }

  if !needs_rebalance(rvolumes, req.kvolumes) {
    return true
  }

  // debug
  fmt.Println("rebalancing", string(req.key), "from", rvolumes, "to", req.kvolumes)

  // always read from the first one
  remote_from := fmt.Sprintf("http://%s%s", rvolumes[0], kp)

  // read
  ss, err := remote_get(remote_from)
  if err != nil {
    fmt.Println("get error", err, remote_from)
    return false
  }

  // write to the kvolumes
  for _, v := range req.kvolumes {
    needs_write := true
    // see if it's already there
    for _, v2 := range rvolumes {
      if v == v2 {
        needs_write = false
        break
      }
    }
    if needs_write {
      remote_to := fmt.Sprintf("http://%s%s", v, kp)
      // write
      if err := remote_put(remote_to, int64(len(ss)), strings.NewReader(ss)); err != nil {
        fmt.Println("put error", err, remote_to)
        return false
      }
    }
  }

  // update db
  if err := db.Put(req.key, fromRecord(Record{req.kvolumes, false}), nil); err != nil {
    fmt.Println("put db error", err)
    return false
  }

  // delete from the volumes that now aren't kvolumes
  delete_error := false
  for _, v2 := range rvolumes {
    needs_delete := true
    for _, v := range req.kvolumes {
      if v == v2 {
        needs_delete = false
        break
      }
    }
    if needs_delete {
      remote_del := fmt.Sprintf("http://%s%s", v2, kp)
      if err := remote_delete(remote_del); err != nil {
        fmt.Println("delete error", err, remote_del)
        delete_error = true
      }
    }
  }
  if delete_error {
    return false
  }
  return true
}

func main() {
  http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost = 100

  volumes := strings.Split(os.Args[1], ",")
  fmt.Println("rebalancing to", volumes)

  db, err := leveldb.OpenFile(os.Args[2], nil)
  if err != nil {
    fmt.Println(fmt.Errorf("LevelDB open failed %s", err))
    return
  }
  defer db.Close()

  var wg sync.WaitGroup
  reqs := make(chan RebalanceRequest, 20000)

  for i := 0; i < 16; i++ {
    go func() {
      for req := range reqs {
        rebalance(db, req)
        wg.Done()
      }
    }()
  }

  iter := db.NewIterator(nil, nil)
  defer iter.Release()
  for iter.Next() {
    key := make([]byte, len(iter.Key()))
    copy(key, iter.Key())
    rec := toRecord(iter.Value())
    kvolumes := key2volume(key, volumes, replicas, subvolumes)
    wg.Add(1)
    reqs <- RebalanceRequest{
      key: key,
      volumes: rec.rvolumes,
      kvolumes: kvolumes}
  }
  close(reqs)

  wg.Wait()
}

