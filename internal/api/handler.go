package api

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

type TreeNode struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Value    string      `json:"value,omitempty"`
	Children []*TreeNode `json:"children,omitempty"`
}

func insertNode(root *TreeNode, parts []string, value string) {
	for i, part := range parts {
		found := false
		for _, child := range root.Children {
			if child.Name == part {
				root = child
				found = true
				break
			}
		}
		if !found {
			newNode := &TreeNode{
				ID:   strings.TrimSuffix(root.ID+"/"+part, "/"),
				Name: part,
			}
			if i == len(parts)-1 {
				newNode.Value = value
			}
			root.Children = append(root.Children, newNode)
			root = newNode
		} else if i == len(parts)-1 {
			root.Value = value
		}
	}
}

// FetchKeysHandler retrieves all keys from etcd.
func FetchKeysHandler(client *clientv3.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c, 5*time.Second)
		defer cancel()

		resp, err := client.Get(ctx, "/", clientv3.WithPrefix(), clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend))
		if err != nil {
			log.Printf("Error fetching keys from etcd: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
			return
		}

		root := &TreeNode{Name: "root"}

		for _, kv := range resp.Kvs {
			keyParts := strings.Split(string(kv.Key), "/")[1:]
			value := string(kv.Value)
			insertNode(root, keyParts, value)
		}
		c.JSON(http.StatusOK, root.Children)
	}
}

// FetchValueForKeyHandler retrieves the value for a specific key from etcd.
func FetchValueForKeyHandler(client *clientv3.Client, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {

		// Retrieve the key with wildcard
		key := c.Param("key")
		if key == "" {
			logger.Error("Key is required")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Key is required"})
			return
		}

		key = strings.TrimPrefix(key, "/")

		// Fetch the value from etcd
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resp, err := client.Get(ctx, key)
		if err != nil {
			logger.Error("Error fetching key from etcd", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
			return
		}

		// If no keys were found, return a not found error
		if len(resp.Kvs) == 0 {
			logger.Info("Key not found", zap.String("key", key))
			c.JSON(http.StatusNotFound, gin.H{"error": "Key not found"})
			return
		}

		// Respond with the value for the key
		kv := resp.Kvs[0]
		value := string(kv.Value)
		c.JSON(http.StatusOK, gin.H{"value": value})
	}
}
