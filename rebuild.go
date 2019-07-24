package main

import (
  "os"
  "fmt"
  "sync"
  "encoding/json"
  "net/http"
  "github.com/syndtr/goleveldb/leveldb"
  "strings"
  "encoding/base64"
  "encoding/hex"
)

type File struct {
  Name string
  Type string
  Mtime string
}

type RebuildRequest struct {
  vol string
  url string
}

var dblock sync.Mutex

func get_files(url string) []File {
  //fmt.Println(url)
  var files []File
  dat, err := remote_get(url)
  if err != nil {
    fmt.Println("ugh", err)
    return files
  }
  json.Unmarshal([]byte(dat), &files)
  return files
}

func rebuild(db *leveldb.DB, volumes []string, req RebuildRequest) bool {
  files := get_files(req.url)
  for _, f := range files {
    key, err := base64.StdEncoding.DecodeString(f.Name)
    if err != nil {
      fmt.Println("ugh", err)
      return false
    }
    kvolumes := key2volume(key, volumes, replicas)

    dblock.Lock()
    data, err := db.Get(key, nil)
    values := make([]string, 0)
    if err != leveldb.ErrNotFound {
      values = strings.Split(string(data), ",")
    }
    values = append(values, req.vol)

    // sort by order in kvolumes (sorry it's n^2 but n is small)
    pvalues := make([]string, 0)
    for _, v := range kvolumes {
      for _, v2 := range values {
        if v == v2 {
          pvalues = append(pvalues, v)
        }
      }
    }
    // insert the ones that aren't there at the end
    for _, v2 := range values {
      insert := true
      for _, v := range kvolumes {
        if v == v2 {
          insert = false
          break
        }
      }
      if insert {
        pvalues = append(pvalues, v2)
      }
    }

    if err := db.Put(key, []byte(strings.Join(pvalues, ",")), nil); err != nil {
      fmt.Println("ugh", err)
      return false
    }
    dblock.Unlock()
    fmt.Println(string(key), pvalues)
  }
  return true
}

func valid(a File) bool {
  if len(a.Name) != 2 || a.Type != "directory" {
    return false
  }
  decoded, err := hex.DecodeString(a.Name)
  if err != nil {
    return false
  }
  if len(decoded) != 1 {
    return false
  }
  return true
}

func main() {
  http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost = 100

  volumes := strings.Split(os.Args[1], ",")
  fmt.Println("rebuilding on", volumes)

  db, err := leveldb.OpenFile(os.Args[2], nil)
  if err != nil {
    fmt.Println(fmt.Errorf("LevelDB open failed %s", err))
    return
  }
  defer db.Close()

  iter := db.NewIterator(nil, nil)
  for iter.Next() {
    db.Delete(iter.Key(), nil)
  }

  var wg sync.WaitGroup
  reqs := make(chan RebuildRequest, 20000)

  for i := 0; i < 128; i++ {
    go func() {
      for req := range reqs {
        rebuild(db, volumes, req)
        wg.Done()
      }
    }()
  }

  parse_volume := func(tvol string) {
    for _, i := range get_files(fmt.Sprintf("http://%s/", tvol)) {
      if valid(i) {
        for _, j := range get_files(fmt.Sprintf("http://%s/%s/", tvol, i.Name)) {
          if valid(j) {
            wg.Add(1)
            url := fmt.Sprintf("http://%s/%s/%s/", tvol, i.Name, j.Name)
            reqs <- RebuildRequest{tvol, url}
          }
        }
      }
    }
  }

  for _, vol := range volumes {
    has_subvolumes := false
    for _, f := range get_files(fmt.Sprintf("http://%s/", vol)) {
      if len(f.Name) == 4 && strings.HasPrefix(f.Name, "sv") && f.Type == "directory" {
        parse_volume(fmt.Sprintf("%s/%s", vol, f.Name))
        has_subvolumes = true
      }
    }
    if !has_subvolumes {
      parse_volume(vol)
    }
  }

  close(reqs)
  wg.Wait()
}

