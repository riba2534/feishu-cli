package client

import (
	"fmt"

	larkdrive "github.com/larksuite/oapi-sdk-go/v3/service/drive/v1"
)

// PermissionMember represents a permission member
type PermissionMember struct {
	MemberType string // "email", "openid", "userid", "unionid", "openchat", "opendepartmentid", "groupid", "wikispaceid"
	MemberID   string
	Perm       string // "view", "edit", "full_access"
}

// AddPermission adds permission to a document
func AddPermission(docToken string, docType string, member PermissionMember, notify bool) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	memberObj := larkdrive.NewBaseMemberBuilder().
		MemberType(member.MemberType).
		MemberId(member.MemberID).
		Perm(member.Perm).
		Build()

	req := larkdrive.NewCreatePermissionMemberReqBuilder().
		Token(docToken).
		Type(docType).
		NeedNotification(notify).
		BaseMember(memberObj).
		Build()

	resp, err := client.Drive.PermissionMember.Create(Context(), req)
	if err != nil {
		return fmt.Errorf("failed to add permission: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("failed to add permission: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// ListPermission lists all permissions for a document
func ListPermission(docToken string, docType string) ([]*larkdrive.Member, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkdrive.NewListPermissionMemberReqBuilder().
		Token(docToken).
		Type(docType).
		Build()

	resp, err := client.Drive.PermissionMember.List(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("failed to list permissions: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("failed to list permissions: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Items, nil
}

// DeletePermission removes permission from a document
func DeletePermission(docToken string, docType string, memberType string, memberID string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larkdrive.NewDeletePermissionMemberReqBuilder().
		Token(docToken).
		Type(docType).
		MemberId(memberID).
		MemberType(memberType).
		Build()

	resp, err := client.Drive.PermissionMember.Delete(Context(), req)
	if err != nil {
		return fmt.Errorf("failed to delete permission: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("failed to delete permission: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// UpdatePublicPermission updates public sharing settings
func UpdatePublicPermission(docToken string, docType string, externalAccess bool, linkShareEntity string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	permPublic := larkdrive.NewPermissionPublicRequestBuilder().
		ExternalAccess(externalAccess).
		LinkShareEntity(linkShareEntity).
		Build()

	req := larkdrive.NewPatchPermissionPublicReqBuilder().
		Token(docToken).
		Type(docType).
		PermissionPublicRequest(permPublic).
		Build()

	resp, err := client.Drive.PermissionPublic.Patch(Context(), req)
	if err != nil {
		return fmt.Errorf("failed to update public permission: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("failed to update public permission: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// UpdatePermission 更新协作者权限
func UpdatePermission(docToken string, docType string, memberID string, memberType string, perm string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larkdrive.NewUpdatePermissionMemberReqBuilder().
		Token(docToken).
		Type(docType).
		MemberId(memberID).
		BaseMember(larkdrive.NewBaseMemberBuilder().
			MemberType(memberType).
			MemberId(memberID).
			Perm(perm).
			Build()).
		Build()

	resp, err := client.Drive.PermissionMember.Update(Context(), req)
	if err != nil {
		return fmt.Errorf("更新权限失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("更新权限失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}
