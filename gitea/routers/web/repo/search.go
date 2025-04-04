// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"net/http"
	"strings"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/modules/base"
	"code.gitea.io/gitea/modules/git"
	code_indexer "code.gitea.io/gitea/modules/indexer/code"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/routers/common"
	"code.gitea.io/gitea/services/context"
)

const tplSearch base.TplName = "repo/search"

func indexSettingToGitGrepPathspecList() (list []string) {
	for _, expr := range setting.Indexer.IncludePatterns {
		list = append(list, ":(glob)"+expr.PatternString())
	}
	for _, expr := range setting.Indexer.ExcludePatterns {
		list = append(list, ":(glob,exclude)"+expr.PatternString())
	}
	return list
}

// Search render repository search page
func Search(ctx *context.Context) {
	ctx.Data["PageIsViewCode"] = true
	prepareSearch := common.PrepareCodeSearch(ctx)
	if prepareSearch.Keyword == "" {
		ctx.HTML(http.StatusOK, tplSearch)
		return
	}

	page := ctx.FormInt("page")
	if page <= 0 {
		page = 1
	}

	var total int
	var searchResults []*code_indexer.Result
	var searchResultLanguages []*code_indexer.SearchResultLanguages
	if setting.Indexer.RepoIndexerEnabled {
		var err error
		total, searchResults, searchResultLanguages, err = code_indexer.PerformSearch(ctx, &code_indexer.SearchOptions{
			RepoIDs:        []int64{ctx.Repo.Repository.ID},
			Keyword:        prepareSearch.Keyword,
			IsKeywordFuzzy: prepareSearch.IsFuzzy,
			Language:       prepareSearch.Language,
			Paginator: &db.ListOptions{
				Page:     page,
				PageSize: setting.UI.RepoSearchPagingNum,
			},
		})
		if err != nil {
			if code_indexer.IsAvailable(ctx) {
				ctx.ServerError("SearchResults", err)
				return
			}
			ctx.Data["CodeIndexerUnavailable"] = true
		} else {
			ctx.Data["CodeIndexerUnavailable"] = !code_indexer.IsAvailable(ctx)
		}
	} else {
		res, err := git.GrepSearch(ctx, ctx.Repo.GitRepo, prepareSearch.Keyword, git.GrepOptions{
			ContextLineNumber: 1,
			IsFuzzy:           prepareSearch.IsFuzzy,
			RefName:           git.RefNameFromBranch(ctx.Repo.BranchName).String(), // BranchName should be default branch or the first existing branch
			PathspecList:      indexSettingToGitGrepPathspecList(),
		})
		if err != nil {
			// TODO: if no branch exists, it reports: exit status 128, fatal: this operation must be run in a work tree.
			ctx.ServerError("GrepSearch", err)
			return
		}
		total = len(res)
		pageStart := min((page-1)*setting.UI.RepoSearchPagingNum, len(res))
		pageEnd := min(page*setting.UI.RepoSearchPagingNum, len(res))
		res = res[pageStart:pageEnd]
		for _, r := range res {
			searchResults = append(searchResults, &code_indexer.Result{
				RepoID:   ctx.Repo.Repository.ID,
				Filename: r.Filename,
				CommitID: ctx.Repo.CommitID,
				// UpdatedUnix: not supported yet
				// Language:    not supported yet
				// Color:       not supported yet
				Lines: code_indexer.HighlightSearchResultCode(r.Filename, "", r.LineNumbers, strings.Join(r.LineCodes, "\n")),
			})
		}
	}

	ctx.Data["Repo"] = ctx.Repo.Repository
	ctx.Data["SearchResults"] = searchResults
	ctx.Data["SearchResultLanguages"] = searchResultLanguages

	pager := context.NewPagination(total, setting.UI.RepoSearchPagingNum, page, 5)
	pager.SetDefaultParams(ctx)
	pager.AddParamString("l", prepareSearch.Language)
	ctx.Data["Page"] = pager

	ctx.HTML(http.StatusOK, tplSearch)
}
