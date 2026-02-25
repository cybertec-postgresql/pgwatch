package sources

import (
	"context"
	"strings"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/jackc/pgx/v5"
)

// TryCreateMissingExtensions attempts to create extensions listed in extensionNames
// if they are not present in existingExtensions. Returns a list of created extensions.
func (md *SourceConn) TryCreateMissingExtensions(
	ctx context.Context,
	extensionNames []string,
	existingExtensions map[string]int,
) []string {

	sqlAvailable := `select name::text from pg_available_extensions`
	extsCreated := make([]string, 0)

	dataRows, err := md.Conn.Query(ctx, sqlAvailable)
	if err != nil {
		log.GetLogger(ctx).Infof("[%s] Failed to get a list of available extensions: %v", md.Name, err)
		return extsCreated
	}
	defer dataRows.Close()

	availableExts := make(map[string]bool)
	var name string

	if _, err := pgx.ForEachRow(dataRows, []any{&name}, func() error {
		availableExts[name] = true
		return nil
	}); err != nil {
		log.GetLogger(ctx).Debugf("[%s] error scanning available extensions: %v", md.Name, err)
	}

	for _, extToCreate := range extensionNames {
		extToCreate = strings.TrimSpace(extToCreate)
		if extToCreate == "" {
			continue
		}

		if _, ok := existingExtensions[extToCreate]; ok {
			continue
		}

		if !availableExts[extToCreate] {
			log.GetLogger(ctx).Errorf(
				"[%s] Requested extension %s not available on instance, cannot try to create...",
				md.Name, extToCreate,
			)
			continue
		}

		// quote identifier safely
		sqlCreateExt := "create extension " + pgx.Identifier{extToCreate}.Sanitize()

		if _, err := md.Conn.Exec(ctx, sqlCreateExt); err != nil {
			log.GetLogger(ctx).Errorf(
				"[%s] Failed to create extension %s (based on --try-create-listed-exts-if-missing input): %v",
				md.Name, extToCreate, err,
			)
			continue
		}

		extsCreated = append(extsCreated, extToCreate)
	}

	return extsCreated
}