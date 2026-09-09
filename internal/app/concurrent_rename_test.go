package app

import (
	"context"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
)

type changedSnapshotClient struct {
	*herdrtest.Client
	change func()
	seen   bool
}

func (c *changedSnapshotClient) Call(
	ctx context.Context,
	method string,
	params any,
	result any,
) error {
	err := c.Client.Call(ctx, method, params, result)
	if method == herdr.MethodSessionSnapshot && !c.seen && err == nil {
		c.seen = true
		c.change()
	}

	return err
}

func TestConcurrentLayoutRenameIsPreserved(t *testing.T) {
	client := herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/project"}},
	)
	changed := &changedSnapshotClient{Client: client, change: func() {
		client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: "Crew"})
	}}
	app := New(testConfig(), discardLogger(), testResolver(t))
	app.manual.Settled()

	for range 2 {
		if err := app.readAndRename(context.Background(), changed); err != nil {
			t.Fatal(err)
		}
	}

	if got := client.Renames(); len(got) != 0 {
		t.Fatalf("overwrote a concurrent layout rename: %v", got)
	}

	if !app.manual.Locked("wE:t1") {
		t.Fatal("the layout's Crew label was not retained as an explicit name")
	}
}

func TestTabClosedAfterSnapshotIsNotRenamed(t *testing.T) {
	client := herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/project"}},
	)
	changed := &changedSnapshotClient{Client: client, change: func() {
		client.CloseTab("wE:t1")
	}}

	app := New(testConfig(), discardLogger(), testResolver(t))
	if err := app.readAndRename(context.Background(), changed); err != nil {
		t.Fatal(err)
	}

	if got := client.Renames(); len(got) != 0 {
		t.Fatalf("renamed a closed tab: %v", got)
	}
}
