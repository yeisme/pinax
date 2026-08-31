package promptrepo

import "context"

type Client interface {
	AddRepository(context.Context, AddRepositoryRequest) (RepositoryView, error)
	ListRepositories(context.Context, ListRepositoriesRequest) (RepositoryPage, error)
	ShowRepository(context.Context, string) (RepositoryView, error)
	RemoveRepository(context.Context, string) error
	SetRepositoryEnabled(context.Context, string, bool) (RepositoryView, error)
	SyncRepositories(context.Context, SyncRequest) (SyncReceipt, error)
	Search(context.Context, SearchRequest) (SearchResult, error)
	Resolve(context.Context, ResolveRequest) (ResolvedSolution, error)
	ReadTemplate(context.Context, ReadTemplateRequest) (TemplateContent, error)
	Stage(context.Context, StageRequest) (StageReceipt, error)
}
