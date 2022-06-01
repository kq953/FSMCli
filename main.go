package main

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"proto"
	"regexp"
	"strings"
	"time"
)

var sendChan chan string

func main() {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Println("err:", err.Error())
		return
	}
	sendChan = make(chan string, 100)
	defer func() {
		w.Close()
		close(sendChan)
	}()

	go send()

	var monitorDir strings.Builder
	monitorDir.WriteString(os.Getenv("GOPATH"))
	monitorDir.WriteString("\\src")

	//遍历目录下的所有目录，将所有目录添加到监听列表
	filepath.Walk(monitorDir.String(), func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
			//隐藏目录忽略
			match, _ := regexp.MatchString(`[/\\]\.[a-zA-Z0-9_]+`, path)
			if match {
				return nil
			}

			path, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			w.Add(path)
		}
		return nil
	})
	//实施监控
	for {
		select {
		case ev := <-w.Events:
			{
				mustSend := false
				fmt.Print(time.Now().Format("2006-01-02 15:04:05"), "\t")
				if ev.Op&fsnotify.Create == fsnotify.Create {
					//文件创建
					//fmt.Println(ev.Name, "created!!")
					//判断是否是文件夹
					info, err := os.Stat(ev.Name)
					if err != nil {
						fmt.Println(err)
					} else {
						//如果是文件夹 添加到侦听列表
						if info.IsDir() {
							w.Add(ev.Name)
						} else {
							mustSend = true
						}
					}
				}
				if ev.Op&fsnotify.Write == fsnotify.Write {
					//文件修改
					mustSend = true
				}
				if ev.Op&fsnotify.Remove == fsnotify.Remove {
					//文件删除
					mustSend = true
				}
				//if ev.Op&fsnotify.Chmod == fsnotify.Chmod {
				//	//修改权限
				//}

				//忽略临时文件
				if mustSend {
					match, _ := regexp.MatchString(`~$`, ev.Name)
					if match {
						mustSend = false
						fmt.Println("临时文件 忽略!!")
					}
				}

				//发送目录地址
				if mustSend {
					proFile := strings.TrimPrefix(ev.Name, monitorDir.String())

					//fmt.Println(proFile)
					r := regexp.MustCompile(`^[/\\]\w+[/\\]`)
					pro := r.FindString(proFile)
					if len(pro) > 2 {
						sendChan <- string([]rune(pro)[1 : len([]rune(pro))-1])
					} else {
						fmt.Println("项目名称获取错误")
					}
				} else {
					fmt.Println("忽略重启")
				}
			}
		case err := <-w.Errors:
			{
				fmt.Println(err)
				return
			}
		}
	}
}

//单独发送进程 防止重复多次发送
func send() {
	var conn net.Conn
	defer func() {
		if conn != nil {
			_ = conn.Close()
		}
	}()
	for {
		fmt.Print(time.Now().Format("2006-01-02 15:04:05"), "\t")
		//连接服务器
	ConnTag:
		conn, err := net.Dial("tcp", "127.0.0.1:8099")
		if err != nil {
			fmt.Println("连接错误:", err.Error())
			time.Sleep(time.Second)
			continue
		} else {
			fmt.Println("服务器连接成功")
		}
		//拿到变更目录
		for {
			proName, ok := <-sendChan
			if !ok {
				continue
			}
			//去除重复
			var proArr = make(map[string]int8)
			proArr[proName] = 1
			time.Sleep(time.Millisecond * 300)
			if l := len(sendChan); l > 0 {
				for i := 0; i < l; i++ {
					proName = <-sendChan
					if _, ok := proArr[proName]; !ok {
						proArr[proName] = 1
					}
				}
			}
			//发送给服务端
			for dir := range proArr {
				var sendStr strings.Builder
				sendStr.WriteString("RERUN:")
				sendStr.WriteString(dir)
				send, err := proto.Encode(sendStr.String())
				if err != nil {
					fmt.Println("创建发送包失败:", err.Error())
				} else {

					n, err := conn.Write(send)
					if err != nil {
						fmt.Println("发送失败：", err.Error())
						time.Sleep(time.Second)
						conn.Close()
						goto ConnTag
					} else {
						fmt.Println("发送成功 长度:", n, " 目录：", dir)
					}
				}
			}
		}
	}
}
