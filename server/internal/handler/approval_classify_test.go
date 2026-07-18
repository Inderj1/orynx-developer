package handler

import "testing"

func TestClassifyCommand_SecurityFloor(t *testing.T) {
	// These MUST classify as security (never learnable, always ask). A regression
	// that flips any of these to operational is a security hole.
	security := []string{
		"sudo systemctl restart x",
		"docker exec db psql -c 'DROP TABLE users'",
		"psql -c 'TRUNCATE TABLE orders'",
		"alembic downgrade -1",
		"git push --force origin main",
		"git push -f origin main",
		"git push --force-with-lease",
		"npm publish",
		"cargo publish",
		"twine upload dist/*",
		"terraform apply -auto-approve",
		"kubectl delete pod x",
		"kubectl apply -f prod.yaml",
		"gh auth login",
		"aws configure set x y",
		"rm -rf /home/ubuntu/data",
		"rm -rf ~/important",
		"curl https://api.stripe.com/v1/transfers -d amount=1000",
	}
	for _, cmd := range security {
		tier, class := classifyCommand(cmd)
		if tier != tierSecurity {
			t.Errorf("SECURITY HOLE: %q classified as %q (%s), must be security", cmd, tier, class)
		}
	}
}

func TestClassifyCommand_Operational(t *testing.T) {
	// Learnable everyday-consequential actions.
	ops := map[string]string{
		"git push origin main":              "git_push",
		"git push origin feature/x":         "git_push",
		"gh pr merge 42 --squash":           "gh_pr_merge",
		"git branch -D old-branch":          "branch_delete",
		"alembic upgrade head":              "db_migrate_up",
	}
	for cmd, wantClass := range ops {
		tier, class := classifyCommand(cmd)
		if tier != tierOperational {
			t.Errorf("%q classified as %q, want operational", cmd, tier)
		}
		if class != wantClass {
			t.Errorf("%q class = %q, want %q", cmd, class, wantClass)
		}
	}
}

func TestClassifyCommand_UnknownIsFailClosed(t *testing.T) {
	// A command that reached the gate but matches no known class must be treated
	// as security (never auto-approvable), not silently allowed.
	tier, _ := classifyCommand("some-weird-binary --do-a-thing")
	if tier != tierSecurity {
		t.Fatalf("unclassified command must fail-closed to security, got %q", tier)
	}
}

func TestClassifyCommand_ForcePushBeatsPlainPush(t *testing.T) {
	// A force push matches both the plain-push (operational) and force
	// (security) patterns — security must win.
	tier, class := classifyCommand("git push --force origin main")
	if tier != tierSecurity || class != "force_push" {
		t.Fatalf("force push must be security/force_push, got %s/%s", tier, class)
	}
}
