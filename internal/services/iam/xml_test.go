package iam_test

import (
	"strings"
	"testing"
	"time"

	iamsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/iam"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const reqID = "req-iam"

func TestIAMUserAndAccessKeyXML(t *testing.T) {
	u := store.User{
		AccountID:  "000000000001",
		UserName:   "alice",
		UserID:     "AIDA123",
		ARN:        "arn:aws:iam::000000000001:user/alice",
		CreateDate: "2024-01-01T00:00:00Z",
	}
	create, err := iamsvc.CreateUserXML(u, reqID)
	if err != nil {
		t.Fatal(err)
	}
	get, err := iamsvc.GetUserXML(u, "arn:aws:iam::000000000001:policy/boundary", reqID)
	if err != nil {
		t.Fatal(err)
	}
	list, err := iamsvc.ListUsersXML([]store.User{u}, reqID)
	if err != nil {
		t.Fatal(err)
	}
	del, err := iamsvc.DeleteUserXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(create), "CreateUserResponse") || !strings.Contains(string(get), "PermissionsBoundary") {
		t.Fatalf("user create=%s get=%s", create, get)
	}
	if !strings.Contains(string(list), "alice") || !strings.Contains(string(del), "DeleteUserResponse") {
		t.Fatalf("list=%s del=%s", list, del)
	}

	cak, err := iamsvc.CreateAccessKeyXML(u.UserName, "AKIA123", "secret", reqID)
	if err != nil {
		t.Fatal(err)
	}
	lak, err := iamsvc.ListAccessKeysXML([]store.AccessKeyMeta{{AccessKeyID: "AKIA123", Status: "Active"}}, reqID)
	if err != nil {
		t.Fatal(err)
	}
	dak, err := iamsvc.DeleteAccessKeyXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	uak, err := iamsvc.UpdateAccessKeyXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cak), "AKIA123") || !strings.Contains(string(lak), "ListAccessKeysResponse") {
		t.Fatalf("keys cak=%s lak=%s", cak, lak)
	}
	if !strings.Contains(string(dak), "DeleteAccessKeyResponse") || !strings.Contains(string(uak), "UpdateAccessKeyResponse") {
		t.Fatalf("dak=%s uak=%s", dak, uak)
	}
}

func TestIAMManagedPolicyXML(t *testing.T) {
	p := store.ManagedPolicy{
		PolicyARN:        "arn:aws:iam::000000000001:policy/Lab",
		AccountID:        "000000000001",
		PolicyName:       "Lab",
		PolicyID:         "ANPA123",
		DefaultVersionID: "v1",
		Document:         `{"Version":"2012-10-17","Statement":[]}`,
	}
	create, err := iamsvc.CreatePolicyXML(p, reqID)
	if err != nil {
		t.Fatal(err)
	}
	get, err := iamsvc.GetPolicyXML(p, reqID)
	if err != nil {
		t.Fatal(err)
	}
	list, err := iamsvc.ListPoliciesXML([]store.ManagedPolicy{p}, reqID)
	if err != nil {
		t.Fatal(err)
	}
	del, err := iamsvc.DeletePolicyXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	ver := store.ManagedPolicyVersion{
		PolicyARN:        p.PolicyARN,
		VersionID:        "v2",
		IsDefaultVersion: true,
		Document:         p.Document,
		CreateDate:       "2024-01-02T00:00:00Z",
	}
	cpv, err := iamsvc.CreatePolicyVersionXML(ver, reqID)
	if err != nil {
		t.Fatal(err)
	}
	gpv, err := iamsvc.GetPolicyVersionXML(ver, reqID)
	if err != nil {
		t.Fatal(err)
	}
	lpv, err := iamsvc.ListPolicyVersionsXML([]store.ManagedPolicyVersion{ver}, reqID)
	if err != nil {
		t.Fatal(err)
	}
	dpv, err := iamsvc.DeletePolicyVersionXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	sdpv, err := iamsvc.SetDefaultPolicyVersionXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(create), "CreatePolicyResponse") || !strings.Contains(string(get), "Lab") {
		t.Fatalf("policy create=%s get=%s", create, get)
	}
	if !strings.Contains(string(list), "ListPoliciesResponse") || !strings.Contains(string(del), "DeletePolicyResponse") {
		t.Fatalf("list=%s del=%s", list, del)
	}
	if !strings.Contains(string(cpv), "v2") || !strings.Contains(string(gpv), "v2") ||
		!strings.Contains(string(lpv), "ListPolicyVersionsResponse") ||
		!strings.Contains(string(dpv), "DeletePolicyVersionResponse") ||
		!strings.Contains(string(sdpv), "SetDefaultPolicyVersionResponse") {
		t.Fatalf("versions cpv=%s gpv=%s lpv=%s dpv=%s sdpv=%s", cpv, gpv, lpv, dpv, sdpv)
	}
}

func TestIAMAttachAndInlinePolicyXML(t *testing.T) {
	ref := []store.AttachedPolicyRef{{PolicyARN: "arn:aws:iam::000000000001:policy/Lab", PolicyName: "Lab"}}
	au, err := iamsvc.AttachUserPolicyXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	du, err := iamsvc.DetachUserPolicyXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	ar, err := iamsvc.AttachRolePolicyXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	dr, err := iamsvc.DetachRolePolicyXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	lau, err := iamsvc.ListAttachedUserPoliciesXML(ref, reqID)
	if err != nil {
		t.Fatal(err)
	}
	lar, err := iamsvc.ListAttachedRolePoliciesXML(ref, reqID)
	if err != nil {
		t.Fatal(err)
	}
	pu, err := iamsvc.PutUserPolicyXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	gu, err := iamsvc.GetUserPolicyXML("alice", "inline", `{"Version":"2012-10-17"}`, reqID)
	if err != nil {
		t.Fatal(err)
	}
	duPol, err := iamsvc.DeleteUserPolicyXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	lu, err := iamsvc.ListUserPoliciesXML([]string{"inline"}, reqID)
	if err != nil {
		t.Fatal(err)
	}
	pr, err := iamsvc.PutRolePolicyXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	gr, err := iamsvc.GetRolePolicyXML("role", "inline", `{"Version":"2012-10-17"}`, reqID)
	if err != nil {
		t.Fatal(err)
	}
	drPol, err := iamsvc.DeleteRolePolicyXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	lr, err := iamsvc.ListRolePoliciesXML([]string{"inline"}, reqID)
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string][]byte{
		"au": au, "du": du, "ar": ar, "dr": dr,
		"lau": lau, "lar": lar, "pu": pu, "gu": gu,
		"duPol": duPol, "lu": lu, "pr": pr, "gr": gr,
		"drPol": drPol, "lr": lr,
	} {
		if !strings.Contains(string(raw), "Response") {
			t.Fatalf("%s missing response: %s", name, raw)
		}
	}
}

func TestIAMRoleXML(t *testing.T) {
	r := store.Role{
		AccountID:          "000000000001",
		RoleName:           "lab-role",
		RoleARN:            "arn:aws:iam::000000000001:role/lab-role",
		TrustPolicy:        `{"Version":"2012-10-17","Statement":[]}`,
		RoleID:             "AROA123",
		CreateDate:         "2024-01-01T00:00:00Z",
		MaxSessionDuration: 7200,
	}
	create, err := iamsvc.CreateRoleXML(r, reqID)
	if err != nil {
		t.Fatal(err)
	}
	get, err := iamsvc.GetRoleXML(r, "arn:aws:iam::000000000001:policy/boundary", reqID)
	if err != nil {
		t.Fatal(err)
	}
	list, err := iamsvc.ListRolesXML([]store.Role{r}, reqID)
	if err != nil {
		t.Fatal(err)
	}
	del, err := iamsvc.DeleteRoleXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	upd, err := iamsvc.UpdateAssumeRolePolicyXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(create), "7200") || !strings.Contains(string(get), "PermissionsBoundary") {
		t.Fatalf("role create=%s get=%s", create, get)
	}
	if !strings.Contains(string(list), "lab-role") || !strings.Contains(string(del), "DeleteRoleResponse") ||
		!strings.Contains(string(upd), "UpdateAssumeRolePolicyResponse") {
		t.Fatalf("list=%s del=%s upd=%s", list, del, upd)
	}
}

func TestIAMGroupAndBoundaryXML(t *testing.T) {
	g := store.Group{
		AccountID: "000000000001",
		GroupName: "devs",
		GroupID:   "AGPA123",
		ARN:       "arn:aws:iam::000000000001:group/devs",
	}
	u := store.User{UserName: "alice", UserID: "AIDA123", ARN: "arn:aws:iam::000000000001:user/alice"}
	createG, err := iamsvc.CreateGroupXML(g, reqID)
	if err != nil {
		t.Fatal(err)
	}
	getG, err := iamsvc.GetGroupXML(g, []store.User{u}, reqID)
	if err != nil {
		t.Fatal(err)
	}
	listG, err := iamsvc.ListGroupsXML([]store.Group{g}, reqID)
	if err != nil {
		t.Fatal(err)
	}
	delG, err := iamsvc.DeleteGroupXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	addU, err := iamsvc.AddUserToGroupXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	rmU, err := iamsvc.RemoveUserFromGroupXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	ag, err := iamsvc.AttachGroupPolicyXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	dg, err := iamsvc.DetachGroupPolicyXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	lag, err := iamsvc.ListAttachedGroupPoliciesXML([]store.AttachedPolicyRef{{PolicyARN: "arn:aws:iam::000000000001:policy/Lab"}}, reqID)
	if err != nil {
		t.Fatal(err)
	}
	pg, err := iamsvc.PutGroupPolicyXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	gg, err := iamsvc.GetGroupPolicyXML("devs", "inline", `{"Version":"2012-10-17"}`, reqID)
	if err != nil {
		t.Fatal(err)
	}
	dgPol, err := iamsvc.DeleteGroupPolicyXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	lg, err := iamsvc.ListGroupPoliciesXML([]string{"inline"}, reqID)
	if err != nil {
		t.Fatal(err)
	}
	puB, err := iamsvc.PutUserPermissionsBoundaryXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	guB, err := iamsvc.GetUserPermissionsBoundaryXML("arn:aws:iam::000000000001:policy/boundary", reqID)
	if err != nil {
		t.Fatal(err)
	}
	duB, err := iamsvc.DeleteUserPermissionsBoundaryXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	prB, err := iamsvc.PutRolePermissionsBoundaryXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	grB, err := iamsvc.GetRolePermissionsBoundaryXML("arn:aws:iam::000000000001:policy/boundary", reqID)
	if err != nil {
		t.Fatal(err)
	}
	drB, err := iamsvc.DeleteRolePermissionsBoundaryXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(createG), "devs") || !strings.Contains(string(getG), "alice") {
		t.Fatalf("group create=%s get=%s", createG, getG)
	}
	for name, raw := range map[string][]byte{
		"listG": listG, "delG": delG, "addU": addU, "rmU": rmU,
		"ag": ag, "dg": dg, "lag": lag, "pg": pg, "gg": gg,
		"dgPol": dgPol, "lg": lg, "puB": puB, "guB": guB,
		"duB": duB, "prB": prB, "grB": grB, "drB": drB,
	} {
		if !strings.Contains(string(raw), "Response") {
			t.Fatalf("%s missing response: %s", name, raw)
		}
	}
}

func TestIAMInstanceProfileAndIdPXML(t *testing.T) {
	ip := store.InstanceProfile{
		AccountID:   "000000000001",
		ProfileName: "lab-profile",
		ProfileARN:  "arn:aws:iam::000000000001:instance-profile/lab-profile",
		RoleName:    "lab-role",
		RoleARN:     "arn:aws:iam::000000000001:role/lab-role",
		RoleID:      "AROA123",
	}
	cip, err := iamsvc.CreateInstanceProfileXML(ip, reqID)
	if err != nil {
		t.Fatal(err)
	}
	dip, err := iamsvc.DeleteInstanceProfileXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	gip, err := iamsvc.GetInstanceProfileXML(ip, reqID)
	if err != nil {
		t.Fatal(err)
	}
	arip, err := iamsvc.AddRoleToInstanceProfileXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	rrip, err := iamsvc.RemoveRoleFromInstanceProfileXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	lip, err := iamsvc.ListInstanceProfilesXML([]store.InstanceProfile{ip}, reqID)
	if err != nil {
		t.Fatal(err)
	}
	lipr, err := iamsvc.ListInstanceProfilesForRoleXML([]store.InstanceProfile{ip}, reqID)
	if err != nil {
		t.Fatal(err)
	}

	oidc := store.OIDCProvider{
		ProviderARN: "arn:aws:iam::000000000001:oidc-provider/token.actions.githubusercontent.com",
		AccountID:   "000000000001",
		URL:         "https://token.actions.githubusercontent.com",
		ClientID:    "sts.amazonaws.com",
		ClientIDs:   []string{"sts.amazonaws.com", "other"},
		Thumbprints: []string{"abc"},
	}
	coidc, err := iamsvc.CreateOpenIDConnectProviderXML(oidc.ProviderARN, reqID)
	if err != nil {
		t.Fatal(err)
	}
	doidc, err := iamsvc.DeleteOpenIDConnectProviderXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	loidc, err := iamsvc.ListOpenIDConnectProvidersXML([]store.OIDCProvider{oidc}, reqID)
	if err != nil {
		t.Fatal(err)
	}
	goidc, err := iamsvc.GetOpenIDConnectProviderXML(oidc, reqID)
	if err != nil {
		t.Fatal(err)
	}

	saml := store.SAMLProvider{
		ProviderARN: "arn:aws:iam::000000000001:saml-provider/lab",
		AccountID:   "000000000001",
		MetadataXML: "<EntityDescriptor/>",
	}
	csaml, err := iamsvc.CreateSAMLProviderXML(saml.ProviderARN, reqID)
	if err != nil {
		t.Fatal(err)
	}
	dsaml, err := iamsvc.DeleteSAMLProviderXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	lsaml, err := iamsvc.ListSAMLProvidersXML([]store.SAMLProvider{saml}, reqID)
	if err != nil {
		t.Fatal(err)
	}
	gsaml, err := iamsvc.GetSAMLProviderXML(saml, reqID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cip), "lab-profile") || !strings.Contains(string(goidc), "ThumbprintList") {
		t.Fatalf("ip=%s oidc=%s", cip, goidc)
	}
	for name, raw := range map[string][]byte{
		"dip": dip, "gip": gip, "arip": arip, "rrip": rrip,
		"lip": lip, "lipr": lipr, "coidc": coidc, "doidc": doidc,
		"loidc": loidc, "csaml": csaml, "dsaml": dsaml,
		"lsaml": lsaml, "gsaml": gsaml,
	} {
		if !strings.Contains(string(raw), "Response") {
			t.Fatalf("%s missing response: %s", name, raw)
		}
	}
}

func TestIAMMFAXML(t *testing.T) {
	cvm, err := iamsvc.CreateVirtualMFADeviceXML("arn:mfa/device", "SEED", reqID)
	if err != nil {
		t.Fatal(err)
	}
	lm, err := iamsvc.ListMFADevicesXML([]store.MFADevice{{Serial: "arn:mfa/device", UserName: "alice"}}, reqID)
	if err != nil {
		t.Fatal(err)
	}
	em, err := iamsvc.EnableMFADeviceXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	dm, err := iamsvc.DeactivateMFADeviceXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cvm), "VirtualMFADevice") || !strings.Contains(string(lm), "MFADevices") {
		t.Fatalf("mfa cvm=%s lm=%s", cvm, lm)
	}
	if !strings.Contains(string(em), "EnableMFADeviceResponse") || !strings.Contains(string(dm), "DeactivateMFADeviceResponse") {
		t.Fatalf("em=%s dm=%s", em, dm)
	}
}

func TestIAMForensicsXML(t *testing.T) {
	used := store.AccessKeyLastUsed{
		UserName:     "alice",
		LastUsedDate: time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
		ServiceName:  "s3",
		Region:       "us-east-1",
		HasLastUsed:  true,
	}
	gak, err := iamsvc.GetAccessKeyLastUsedXML(used, reqID)
	if err != nil {
		t.Fatal(err)
	}
	gen, err := iamsvc.GenerateCredentialReportXML("STARTED", reqID)
	if err != nil {
		t.Fatal(err)
	}
	get, err := iamsvc.GetCredentialReportXML([]byte("user,arn"), used.LastUsedDate, "COMPLETE", reqID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gak), "s3") || !strings.Contains(string(gen), "STARTED") {
		t.Fatalf("gak=%s gen=%s", gak, gen)
	}
	if !strings.Contains(string(get), "text/csv") || !strings.Contains(string(get), "Content") {
		t.Fatalf("get=%s", get)
	}
}
