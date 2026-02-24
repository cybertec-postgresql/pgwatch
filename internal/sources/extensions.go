package sources

import (
	"context"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/jackc/pgx/v5"
)

// TryCreateMissingExtensions attempts to create extensions listed in extensionNames
// if they are not present in existingExtensions. Returns a list of created extensions.
func TryCreateMissingExtensions(ctx context.Context, md *SourceConn, extensionNames []string, existingExtensions map[string]int) []string {
    // For security reasons don't allow to execute random strings but check that it's an existing extension
    sqlAvailable := `select name::text from pg_available_extensions`
    extsCreated := make([]string, 0)

    dataRows, err := md.Conn.Query(ctx, sqlAvailable)
    if err != nil {
        log.GetLogger(ctx).Infof("[%s] Failed to get a list of available extensions: %v", md, err)
        return extsCreated
    }
    defer dataRows.Close()

    availableExts := make(map[string]bool)
    var name string
    if _, err := pgx.ForEachRow(dataRows, []any{&name}, func() error {
        availableExts[name] = true
        return nil
    }); err != nil {
        // ignore iteration error but log it
        log.GetLogger(ctx).Debugf("[%s] error scanning available extensions: %v", md, err)
    }

    for _, extToCreate := range extensionNames {
        if _, ok := existingExtensions[extToCreate]; ok {
            continue
        }
        _, ok := availableExts[extToCreate]
        if !ok {
            log.GetLogger(ctx).Errorf("[%s] Requested extension %s not available on instance, cannot try to create...", md, extToCreate)
        } else {
            sqlCreateExt := `create extension ` + extToCreate
            if _, err := md.Conn.Exec(ctx, sqlCreateExt); err != nil {
                log.GetLogger(ctx).Errorf("[%s] Failed to create extension %s (based on --try-create-listed-exts-if-missing input): %v", md, extToCreate, err)
            }
            extsCreated = append(extsCreated, extToCreate)
        }
    }

    return extsCreated
}
