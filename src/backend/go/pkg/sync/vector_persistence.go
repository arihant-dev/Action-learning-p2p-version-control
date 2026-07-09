package sync

import (
	"encoding/json"
	"fmt"

	"p2p/pkg/versioning"
)

func serializeVectorClock(vc *versioning.VectorClock) (string, error) {
	if vc == nil {
		return "{}", nil
	}
	data, err := json.Marshal(vc.AsMap())
	if err != nil {
		return "{}", err
	}
	return string(data), nil
}

func deserializeVectorClock(data string) (*versioning.VectorClock, error) {
	if data == "" || data == "{}" {
		return versioning.NewVectorClock(), nil
	}
	vc := versioning.NewVectorClock()
	m := make(map[string]uint64)
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		return nil, fmt.Errorf("deserialize vector clock: %w", err)
	}
	vc.Merge(m)
	return vc, nil
}

func (sc *SyncCoordinator) loadVectorClocks() error {
	repos, err := sc.db.Repositories().List()
	if err != nil {
		return fmt.Errorf("list repos for vector clock loading: %w", err)
	}

	for _, repo := range repos {
		files, err := sc.db.Metadata().ListByRepository(repo.ID, true)
		if err != nil {
			return fmt.Errorf("list metadata for repo %s: %w", repo.ID, err)
		}
		for _, f := range files {
			if f.VectorClock != "" {
				vc, err := deserializeVectorClock(f.VectorClock)
				if err != nil {
					zlog.Warn().Err(err).Str("repo_id", repo.ID).Str("file", f.Filepath).Msg("Failed to deserialize vector clock")
					continue
				}
				sc.vectorClock.Merge(vc.AsMap())
			}
		}
	}
	return nil
}

func (sc *SyncCoordinator) persistVectorClock(repoID, filepath string, vc *versioning.VectorClock) error {
	vcJSON, err := serializeVectorClock(vc)
	if err != nil {
		return err
	}

	meta, err := sc.db.Metadata().Get(repoID, filepath)
	if err != nil || meta == nil {
		return nil
	}

	meta.VectorClock = vcJSON
	return sc.db.Metadata().Save(meta)
}

func (sc *SyncCoordinator) PersistAllVectorClocks() error {
	repos, err := sc.db.Repositories().List()
	if err != nil {
		return fmt.Errorf("list repos: %w", err)
	}

	for _, repo := range repos {
		files, err := sc.db.Metadata().ListByRepository(repo.ID, false)
		if err != nil {
			zlog.Warn().Err(err).Str("repo_id", repo.ID).Msg("Failed to list metadata for repo")
			continue
		}
		for _, f := range files {
			vcJSON, err := serializeVectorClock(sc.vectorClock)
			if err != nil {
				continue
			}
			f.VectorClock = vcJSON
			if err := sc.db.Metadata().Save(f); err != nil {
				zlog.Warn().Err(err).Str("repo_id", repo.ID).Str("file", f.Filepath).Msg("Failed to save vector clock")
			}
		}
	}
	return nil
}
