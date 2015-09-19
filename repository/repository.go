package repository

import (
	"fmt"
	"strings"

	"github.com/megamsys/megamd/meta"
)

const (
	defaultManager = "github"
	CI             = "CI"
	CI_ENABLED     = "ci-enabled"
	CI_TOKEN       = "ci-token"
	CI_SCM         = "ci-scm"
	CI_USER        = "ci-user"
	CI_URL         = "ci-url"
	CI_APIVERSION  = "ci-apiversion"
)

var managers map[string]RepositoryManager

/* Repository represents a repository managed by the manager. */
type Repo struct {
	Enabled  bool
	Token    string
	Git      string
	UserName string
	CartonId string
	BoxId    string
}

func (r Repo) IsEnabled() bool {
	return r.Enabled
}

func (r Repo) GetToken() string {
	return r.Token
}

func (r Repo) Gitr() string {
	return r.Git
}

func (r Repo) Trigger() string {
	return meta.MC.Api + "/assembly/build/" + r.CartonId + "/" + r.BoxId
}

func (r Repo) GetUserName() string {
	return r.UserName
}

func (r Repo) GetShortName(fullgit_url string) (string, error) {
	i := strings.LastIndex(fullgit_url, "/")
	if i < 0 {
		return "", fmt.Errorf("unable to parse output of git")
	}
	return fullgit_url[i+1:], nil
}

type Repository interface {
	IsEnabled() bool
	GetToken() string
	Gitr() string
	Trigger() string
	GetUserName() string
	GetShortName() (string, error)
}

// RepositoryManager represents a manager of application repositories.
type RepositoryManager interface {
	CreateHook(r Repository) (string, error)
	RemoveHook(r Repository) error
}

// Manager returns the current configured manager, as defined in the
// configuration file.
func Manager(managerName string) RepositoryManager {
	if _, ok := managers[managerName]; !ok {
		managerName = "nop"
	}
	return managers[managerName]
}

// Register registers a new repository manager, that can be later configured
// and used.
func Register(name string, manager RepositoryManager) {
	if managers == nil {
		managers = make(map[string]RepositoryManager)
	}
	managers[name] = manager
}