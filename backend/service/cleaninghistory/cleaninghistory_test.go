package cleaninghistory

import (
	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	settingmodel "ai-disk-cleanner/backend/data/models/setting"
)

type fakeStore struct {
	settings   []settingmodel.Setting
	calls      []string
	deletedMax int
	records    []cleaningrecord.CleaningRecord
	deleted    []int64
	deleteErr  error
}

func (store *fakeStore) ListSettings(context.Context) ([]settingmodel.Setting, error) {
	store.calls = append(store.calls, "list")
	return store.settings, nil
}

func (store *fakeStore) MarkInterruptedCleaningRecords(context.Context) error {
	store.calls = append(store.calls, "mark")
	return nil
}

func (store *fakeStore) ListOldCleaningRecords(_ context.Context, maxCount int) ([]cleaningrecord.CleaningRecord, error) {
	store.calls = append(store.calls, "candidates")
	store.deletedMax = maxCount
	return store.records, nil
}

func (store *fakeStore) DeleteCleaningRecordsByIDs(_ context.Context, ids []int64) error {
	store.calls = append(store.calls, "delete")
	store.deleted = ids
	return store.deleteErr
}

func TestCleanupOnStartupUsesConfiguredRecordLimit(t *testing.T) {
	store := &fakeStore{settings: []settingmodel.Setting{
		{Key: recordMaxCountKey, Value: "7"},
	}}
	service := newService(context.Background(), store)

	if err := service.CleanupOnStartup(); err != nil {
		t.Fatalf("CleanupOnStartup() error = %v", err)
	}
	if store.deletedMax != 7 {
		t.Fatalf("deleted max = %d, want 7", store.deletedMax)
	}
	if want := []string{"mark", "list", "candidates", "delete"}; !reflect.DeepEqual(store.calls, want) {
		t.Fatalf("calls = %#v, want %#v", store.calls, want)
	}
}

func TestCleanupSkipsOnlyFailedLogAndReturnsDatabaseFailure(t *testing.T) {
	for _, databaseFails := range []bool{false, true} {
		t.Run(fmtBool(databaseFails), func(t *testing.T) {
			store := &fakeStore{settings: []settingmodel.Setting{{Key: recordMaxCountKey, Value: "2"}}}
			for i := int64(1); i <= 3; i++ {
				store.records = append(store.records, cleaningrecord.CleaningRecord{ID: i, StartTime: time.Unix(i, 0)})
			}
			if databaseFails {
				store.deleteErr = errors.New("database unavailable")
			}
			service := newService(context.Background(), store)
			var attempted []int64
			service.removeLog = func(start time.Time) error {
				attempted = append(attempted, start.Unix())
				if len(store.deleted) != 0 {
					t.Fatal("database deleted before logs")
				}
				if start.Unix() == 2 {
					return errors.New("file busy")
				}
				return nil
			}
			err := service.CleanupOnStartup()
			if !errors.Is(err, store.deleteErr) {
				t.Fatalf("error = %v", err)
			}
			if !reflect.DeepEqual(attempted, []int64{1, 2, 3}) || !reflect.DeepEqual(store.deleted, []int64{1, 3}) {
				t.Fatalf("attempted=%v deleted=%v", attempted, store.deleted)
			}
		})
	}
}

func fmtBool(value bool) string {
	if value {
		return "database_failure"
	}
	return "success"
}

func TestCleanupOnStartupRejectsInvalidRecordLimit(t *testing.T) {
	store := &fakeStore{settings: []settingmodel.Setting{
		{Key: recordMaxCountKey, Value: "invalid"},
	}}
	service := newService(context.Background(), store)

	if err := service.CleanupOnStartup(); err == nil {
		t.Fatal("CleanupOnStartup() error = nil, want validation error")
	}
	if store.deletedMax != 0 {
		t.Fatalf("DeleteOldCleaningRecords() called with %d", store.deletedMax)
	}
}
