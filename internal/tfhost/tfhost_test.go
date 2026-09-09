package tfhost

import "testing"

func TestEnvName(t *testing.T) {
	cases := []struct {
		host string
		want string
	}{
		{"app.terraform.io", "TF_TOKEN_app_terraform_io"},
		{"App.Terraform.IO", "TF_TOKEN_app_terraform_io"},
		{"tfe.example-corp.com", "TF_TOKEN_tfe_example__corp_com"},
		{"例え.com", "TF_TOKEN_xn____r8jz45g_com"},
	}
	for _, c := range cases {
		got, err := EnvName(c.host)
		if err != nil {
			t.Errorf("EnvName(%q) returned %v", c.host, err)
			continue
		}
		if got != c.want {
			t.Errorf("EnvName(%q) = %q, want %q", c.host, got, c.want)
		}
	}
}

func TestEnvNameRejects(t *testing.T) {
	for _, host := range []string{"", "https://app.terraform.io", "localhost:8080"} {
		if _, err := EnvName(host); err == nil {
			t.Errorf("EnvName(%q) should have failed", host)
		}
	}
}

func TestNormalize(t *testing.T) {
	got, err := Normalize("  App.Terraform.IO ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "app.terraform.io" {
		t.Fatalf("Normalize = %q", got)
	}
}
