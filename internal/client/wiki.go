package client

import (
	"context"
	"fmt"

	larkwiki "github.com/larksuite/oapi-sdk-go/v3/service/wiki/v2"
)

// WikiNode 知识库节点信息
type WikiNode struct {
	SpaceID         string `json:"space_id"`
	NodeToken       string `json:"node_token"`
	ObjToken        string `json:"obj_token"`
	ObjType         string `json:"obj_type"`
	ParentNodeToken string `json:"parent_node_token,omitempty"`
	NodeType        string `json:"node_type"`
	Title           string `json:"title"`
	HasChild        bool   `json:"has_child"`
	Creator         string `json:"creator,omitempty"`
	Owner           string `json:"owner,omitempty"`
	ObjCreateTime   string `json:"obj_create_time,omitempty"`
	ObjEditTime     string `json:"obj_edit_time,omitempty"`
}

// WikiSpace 知识空间信息
type WikiSpace struct {
	SpaceID     string `json:"space_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	SpaceType   string `json:"space_type,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
}

// GetWikiNode 获取知识库节点信息
func GetWikiNode(token string) (*WikiNode, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkwiki.NewGetNodeSpaceReqBuilder().
		Token(token).
		Build()

	resp, err := client.Wiki.Space.GetNode(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("获取节点信息失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取节点信息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	node := resp.Data.Node
	if node == nil {
		return nil, fmt.Errorf("节点不存在")
	}

	result := &WikiNode{
		NodeToken: token,
	}

	if node.SpaceId != nil {
		result.SpaceID = *node.SpaceId
	}
	if node.ObjToken != nil {
		result.ObjToken = *node.ObjToken
	}
	if node.ObjType != nil {
		result.ObjType = *node.ObjType
	}
	if node.ParentNodeToken != nil {
		result.ParentNodeToken = *node.ParentNodeToken
	}
	if node.NodeType != nil {
		result.NodeType = *node.NodeType
	}
	if node.Title != nil {
		result.Title = *node.Title
	}
	if node.HasChild != nil {
		result.HasChild = *node.HasChild
	}
	if node.Creator != nil {
		result.Creator = *node.Creator
	}
	if node.Owner != nil {
		result.Owner = *node.Owner
	}
	if node.ObjCreateTime != nil {
		result.ObjCreateTime = *node.ObjCreateTime
	}
	if node.ObjEditTime != nil {
		result.ObjEditTime = *node.ObjEditTime
	}

	return result, nil
}

// ListWikiSpaces 获取知识空间列表
func ListWikiSpaces(pageSize int, pageToken string) ([]*WikiSpace, string, bool, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", false, err
	}

	reqBuilder := larkwiki.NewListSpaceReqBuilder()
	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Wiki.Space.List(context.Background(), reqBuilder.Build())
	if err != nil {
		return nil, "", false, fmt.Errorf("获取知识空间列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, "", false, fmt.Errorf("获取知识空间列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var spaces []*WikiSpace
	if resp.Data != nil && resp.Data.Items != nil {
		for _, item := range resp.Data.Items {
			space := &WikiSpace{}
			if item.SpaceId != nil {
				space.SpaceID = *item.SpaceId
			}
			if item.Name != nil {
				space.Name = *item.Name
			}
			if item.Description != nil {
				space.Description = *item.Description
			}
			if item.SpaceType != nil {
				space.SpaceType = *item.SpaceType
			}
			if item.Visibility != nil {
				space.Visibility = *item.Visibility
			}
			spaces = append(spaces, space)
		}
	}

	var nextPageToken string
	var hasMore bool
	if resp.Data != nil {
		if resp.Data.PageToken != nil {
			nextPageToken = *resp.Data.PageToken
		}
		if resp.Data.HasMore != nil {
			hasMore = *resp.Data.HasMore
		}
	}

	return spaces, nextPageToken, hasMore, nil
}

// ListWikiNodes 获取知识空间下的节点列表
func ListWikiNodes(spaceID string, parentNodeToken string, pageSize int, pageToken string) ([]*WikiNode, string, bool, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", false, err
	}

	reqBuilder := larkwiki.NewListSpaceNodeReqBuilder().
		SpaceId(spaceID)

	if parentNodeToken != "" {
		reqBuilder.ParentNodeToken(parentNodeToken)
	}
	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Wiki.SpaceNode.List(context.Background(), reqBuilder.Build())
	if err != nil {
		return nil, "", false, fmt.Errorf("获取节点列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, "", false, fmt.Errorf("获取节点列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var nodes []*WikiNode
	if resp.Data != nil && resp.Data.Items != nil {
		for _, item := range resp.Data.Items {
			node := &WikiNode{}
			if item.SpaceId != nil {
				node.SpaceID = *item.SpaceId
			}
			if item.NodeToken != nil {
				node.NodeToken = *item.NodeToken
			}
			if item.ObjToken != nil {
				node.ObjToken = *item.ObjToken
			}
			if item.ObjType != nil {
				node.ObjType = *item.ObjType
			}
			if item.ParentNodeToken != nil {
				node.ParentNodeToken = *item.ParentNodeToken
			}
			if item.NodeType != nil {
				node.NodeType = *item.NodeType
			}
			if item.Title != nil {
				node.Title = *item.Title
			}
			if item.HasChild != nil {
				node.HasChild = *item.HasChild
			}
			if item.Creator != nil {
				node.Creator = *item.Creator
			}
			if item.Owner != nil {
				node.Owner = *item.Owner
			}
			nodes = append(nodes, node)
		}
	}

	var nextPageToken string
	var hasMore bool
	if resp.Data != nil {
		if resp.Data.PageToken != nil {
			nextPageToken = *resp.Data.PageToken
		}
		if resp.Data.HasMore != nil {
			hasMore = *resp.Data.HasMore
		}
	}

	return nodes, nextPageToken, hasMore, nil
}

// CreateWikiNodeResult 创建节点的结果
type CreateWikiNodeResult struct {
	SpaceID   string `json:"space_id"`
	NodeToken string `json:"node_token"`
	ObjToken  string `json:"obj_token"`
	ObjType   string `json:"obj_type"`
}

// CreateWikiNode 在知识空间中创建节点
func CreateWikiNode(spaceID, title, parentNode, nodeType string) (*CreateWikiNodeResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	// 构建节点信息
	nodeBuilder := larkwiki.NewNodeBuilder().
		Title(title)

	// 设置节点类型，默认为 docx
	if nodeType == "" {
		nodeType = larkwiki.ObjTypeObjTypeDocx
	}
	nodeBuilder.ObjType(nodeType)

	// 设置父节点
	if parentNode != "" {
		nodeBuilder.ParentNodeToken(parentNode)
	}

	req := larkwiki.NewCreateSpaceNodeReqBuilder().
		SpaceId(spaceID).
		Node(nodeBuilder.Build()).
		Build()

	resp, err := client.Wiki.SpaceNode.Create(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("创建知识库节点失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("创建知识库节点失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil || resp.Data.Node == nil {
		return nil, fmt.Errorf("创建节点成功但未返回节点信息")
	}

	result := &CreateWikiNodeResult{}
	node := resp.Data.Node

	if node.SpaceId != nil {
		result.SpaceID = *node.SpaceId
	}
	if node.NodeToken != nil {
		result.NodeToken = *node.NodeToken
	}
	if node.ObjToken != nil {
		result.ObjToken = *node.ObjToken
	}
	if node.ObjType != nil {
		result.ObjType = *node.ObjType
	}

	return result, nil
}

// UpdateWikiNode 更新知识库节点标题
func UpdateWikiNode(spaceID, nodeToken, title string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	body := larkwiki.NewUpdateTitleSpaceNodeReqBodyBuilder().
		Title(title).
		Build()

	req := larkwiki.NewUpdateTitleSpaceNodeReqBuilder().
		SpaceId(spaceID).
		NodeToken(nodeToken).
		Body(body).
		Build()

	resp, err := client.Wiki.SpaceNode.UpdateTitle(context.Background(), req)
	if err != nil {
		return fmt.Errorf("更新知识库节点标题失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("更新知识库节点标题失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// MoveWikiNodeResult 移动节点的结果
type MoveWikiNodeResult struct {
	NodeToken string `json:"node_token"`
}

// MoveWikiNode 移动知识库节点
func MoveWikiNode(spaceID, nodeToken, targetSpaceID, targetParent string) (*MoveWikiNodeResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	bodyBuilder := larkwiki.NewMoveSpaceNodeReqBodyBuilder().
		TargetSpaceId(targetSpaceID)

	if targetParent != "" {
		bodyBuilder.TargetParentToken(targetParent)
	}

	req := larkwiki.NewMoveSpaceNodeReqBuilder().
		SpaceId(spaceID).
		NodeToken(nodeToken).
		Body(bodyBuilder.Build()).
		Build()

	resp, err := client.Wiki.SpaceNode.Move(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("移动知识库节点失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("移动知识库节点失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	result := &MoveWikiNodeResult{}
	if resp.Data != nil && resp.Data.Node != nil && resp.Data.Node.NodeToken != nil {
		result.NodeToken = *resp.Data.Node.NodeToken
	}

	return result, nil
}
