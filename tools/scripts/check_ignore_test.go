package scripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckIgnore(t *testing.T) {
	gitignore, err := os.ReadFile(repositoryFile(".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	// without returns the repository's .gitignore without the line pattern.
	without := func(t *testing.T, pattern string) string {
		t.Helper()
		lines := strings.Split(string(gitignore), "\n")
		kept := lines[:0]
		for _, line := range lines {
			if line != pattern {
				kept = append(kept, line)
			}
		}
		if len(kept) == len(lines) {
			t.Fatalf(".gitignore has no line %q", pattern)
		}
		return strings.Join(kept, "\n")
	}
	tests := []struct {
		name string
		// gitignore is the content of .gitignore at the root, which noGitignore leaves out.
		gitignore   string
		noGitignore bool
		// files are written into the repository before it is created, and noRepository leaves
		// it out. forceAdd names files that git add -f then adds although they are ignored.
		files        map[string]string
		noRepository bool
		forceAdd     []string
		// stubGit, when it is not empty, is a program that the script finds as git on PATH.
		stubGit string
		args    []string
		// globalExcludes is the content of the excludes file that the global git configuration
		// names, when it is not empty.
		globalExcludes string
		wantExit       int
		wantStderr     []string
	}{
		{
			name: "the repository's .gitignore", gitignore: string(gitignore), wantExit: 0,
			wantStderr: []string{"check-ignore: checked 16 ignored and 3 committable paths"},
		},
		{
			name:      "a credential file that git tracks",
			gitignore: string(gitignore), files: map[string]string{".env": "x\n"},
			forceAdd: []string{".env"}, wantExit: 0,
			wantStderr: []string{"check-ignore: checked 16 ignored and 3 committable paths"},
		},
		{
			name: "a pattern removed", gitignore: without(t, "*.pem"), wantExit: 1,
			wantStderr: []string{
				"check-ignore: git does not ignore a.pem; add a pattern for it to .gitignore",
				"check-ignore: git does not ignore sub/b.pem;",
			},
		},
		{
			name:      "a pattern removed and set in the global excludes file",
			gitignore: without(t, "*.pem"), globalExcludes: "*.pem\n", wantExit: 1,
			wantStderr: []string{"check-ignore: git does not ignore a.pem;"},
		},
		{
			name: "the exception for .env.example removed", gitignore: without(t, "!.env.example"),
			wantExit: 1,
			wantStderr: []string{
				"check-ignore: git ignores .env.example because of .gitignore:",
				":.env*; it must stay committable",
				"check-ignore: git ignores sub/.env.example because of .gitignore:",
			},
		},
		{
			name: "a pattern that ignores too much", gitignore: string(gitignore) + "bin/\n",
			wantExit: 1,
			wantStderr: []string{"check-ignore: git ignores sub/bin/tool because of " +
				".gitignore:"},
		},
		{
			name:      "a .gitignore in a directory that brings a file back",
			gitignore: string(gitignore), files: map[string]string{"sub/.gitignore": "!*.pem\n"},
			wantExit: 1, wantStderr: []string{"check-ignore: git does not ignore sub/b.pem;"},
		},
		{
			name: "an empty .gitignore", gitignore: "", wantExit: 1,
			wantStderr: []string{"check-ignore: git does not ignore .env;"},
		},
		{
			name: "no .gitignore", noGitignore: true, wantExit: 2,
			wantStderr: []string{"check-ignore: the repository root has no .gitignore"},
		},
		{
			name:         "not a git repository",
			gitignore:    string(gitignore),
			noRepository: true,
			wantExit:     2,
			wantStderr:   []string{"check-ignore: git check-ignore failed with exit code"},
		},
		{
			name:      "a git check-ignore that answers for fewer paths than it was given",
			gitignore: string(gitignore),
			stubGit:   "#!/bin/sh\ncat >/dev/null\nprintf '.gitignore:1:.env\\t.env\\n'\n",
			wantExit:  2,
			wantStderr: []string{
				"check-ignore: git check-ignore answered for 1 of 19 paths",
			},
		},
		{
			name: "arguments", gitignore: string(gitignore), args: []string{".env"}, wantExit: 2,
			wantStderr: []string{"check-ignore: usage: check-ignore.sh (it takes no arguments)"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			copyScripts(t, root, "check-ignore.sh")
			writeFiles(t, root, tt.files)
			if !tt.noGitignore {
				writeFiles(t, root, map[string]string{".gitignore": tt.gitignore})
			}
			if !tt.noRepository {
				gitInit(t, root)
			}
			if len(tt.forceAdd) > 0 {
				args := append([]string{"add", "-f", "--"}, tt.forceAdd...)
				if result := runScript(t, root, gitEnv(root), "git", args...); result.exit != 0 {
					t.Fatalf("git add -f exited %d: %s", result.exit, result.stderr)
				}
			}
			env := gitEnv(root)
			if tt.stubGit != "" {
				bin := t.TempDir()
				writeFiles(t, bin, map[string]string{"git": tt.stubGit})
				if err := os.Chmod(filepath.Join(bin, "git"), 0o755); err != nil {
					t.Fatal(err)
				}
				inherited := os.Getenv("PATH") //nolint:forbidigo // the stub goes before it
				env = append(env, "PATH="+bin+string(os.PathListSeparator)+inherited)
			}
			if tt.globalExcludes != "" {
				home := t.TempDir()
				excludes := filepath.Join(home, "excludes")
				config := filepath.Join(home, "gitconfig")
				writeFiles(t, home, map[string]string{
					"excludes":  tt.globalExcludes,
					"gitconfig": "[core]\n\texcludesFile = " + excludes + "\n",
				})
				env = append(env, "GIT_CONFIG_GLOBAL="+config)
			}

			result := runScript(t, root, env,
				filepath.Join(root, ".github", "scripts", "check-ignore.sh"), tt.args...)

			if result.exit != tt.wantExit {
				t.Errorf("exit = %d, want %d; stderr:\n%s", result.exit, tt.wantExit, result.stderr)
			}
			for _, want := range tt.wantStderr {
				if !strings.Contains(result.stderr, want) {
					t.Errorf("stderr does not contain %q:\n%s", want, result.stderr)
				}
			}
			if result.stdout != "" {
				t.Errorf("stdout = %q, want nothing", result.stdout)
			}
		})
	}
}
