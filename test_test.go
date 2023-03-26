package main

import (
	"7daysgo/web"
	"fmt"
	"net/http"
	"testing"
)

func TestWeb(t *testing.T) {
	r := web.New()
	r.GET("/", func(c *web.Context) {
		c.HTML(http.StatusOK, "<h1>Hello Gee</h1>")
	})
	r.GET("/hello", func(c *web.Context) {
		// expect /hello?name=geektutu
		c.String(http.StatusOK, "hello %s, you're at %s\n", c.Query("name"), c.Path)
	})

	r.POST("/login", func(c *web.Context) {
		c.JSON(http.StatusOK, web.H{
			"username": c.PostForm("username"),
			"password": c.PostForm("password"),
		})
	})

	r.Run(":9999")
}

func TestCache(t *testing.T) {
	obj := cache.MakeLFU(1)
	fmt.Println(obj.Get(1))
	obj.Put(1, 1)
	obj.Put(2, 1)
	obj.Put(3, 1)
	fmt.Println(obj.Get(1))
	fmt.Println(obj.Get(3))
}
