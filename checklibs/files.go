// ********************************************************************
// The file system status check module (directories and files).
// It implements the verification of changes to the specified files
// and directories (modification, appearance of new ones).
// ********************************************************************

package checklibs

import (
    "fmt"
    "os"
    "path/filepath"
    "io/fs"
    "os/user"
    "syscall"
    "time"
    "sort"
    "maps"
    "strings"
    "log"

    "watchdog/mainlibs"
)

// File/Directory metadata
type FileMeta struct {
	Path     string
	FileType string
	Size     int64
	Mode     fs.FileMode
	Owner    string
	Group    string
	ModTime  time.Time
	// CreateTime time.Time
}

// FileMap map[fullpath] → metadata
type FileMap map[string]FileMeta

// FieldChange — one changed field
type FieldChange struct {
	Field string
	Old   string
	New   string
}

// Changes of one file
type FileDiff struct {
	Path    string
	Changes []FieldChange
}

// Full diff result
type DiffResult struct {
	OnlyInBase   []string   // deleted
	OnlyInActual []string   // added
	Changed      []FileDiff // changes
	Unchanged    []string
}


func PrintDiff(d DiffResult, showUnchanged bool) string {
	var sb strings.Builder

	sort.Strings(d.OnlyInBase)
	sort.Strings(d.OnlyInActual)

    hostname, err := mainlibs.GetFQDN()
    if err != nil {
        hostname = "unknow"
        log.Println("Hostname error:", err)
    }

    fmt.Fprintf(&sb, "File changes alarm on host %s:\n", hostname)

	if len(d.OnlyInBase) > 0 {
		fmt.Fprintf(&sb, "\nDeleted (%d):\n", len(d.OnlyInBase))
		for _, p := range d.OnlyInBase {
			fmt.Fprintf(&sb, "   - %s\n", p)
		}
	}

	if len(d.OnlyInActual) > 0 {
		fmt.Fprintf(&sb, "\nAdded (%d):\n", len(d.OnlyInActual))
		for _, p := range d.OnlyInActual {
			fmt.Fprintf(&sb, "   + %s\n", p)
		}
	}

	if len(d.Changed) > 0 {
		fmt.Fprintf(&sb, "\nChanged (%d):\n", len(d.Changed))
		sort.Slice(d.Changed, func(i, j int) bool {
			return d.Changed[i].Path < d.Changed[j].Path
		})
		for _, fd := range d.Changed {
			fmt.Fprintf(&sb, "   ~ %s\n", fd.Path)
			for _, ch := range fd.Changes {
				fmt.Fprintf(&sb, "       %-6s: %q → %q\n", ch.Field, ch.Old, ch.New)
			}
		}
	}

	if showUnchanged && len(d.Unchanged) > 0 {
		fmt.Fprintf(&sb, "\n✓ Unchanged (%d):\n", len(d.Unchanged))
		sort.Strings(d.Unchanged)
		for _, p := range d.Unchanged {
			fmt.Fprintf(&sb, "   = %s\n", p)
		}
	}

	fmt.Fprintf(&sb, "\nSummary: deleted %d, added %d, changed %d, all %d\n",
		len(d.OnlyInBase), len(d.OnlyInActual),
		len(d.Changed), len(d.Unchanged))

    if len(d.OnlyInBase) > 0 || len(d.OnlyInActual) > 0 || len(d.Changed) > 0 {
	    return sb.String()
    } else {
        return ""
    }
}

// Compares the file lists from the previous and current checks
func CompareFilesList(base, actual FileMap) DiffResult {
	var res DiffResult

	for path, b := range base {
		a, ok := actual[path]
		if !ok {
			res.OnlyInBase = append(res.OnlyInBase, path)
			continue
		}

		var changes []FieldChange

		if b.FileType != a.FileType {
			changes = append(changes, FieldChange{"FileType", b.FileType, a.FileType})
		}
		if b.Size != a.Size {
			changes = append(changes, FieldChange{"size",
				fmt.Sprint(b.Size), fmt.Sprint(a.Size)})
		}
		if b.Mode.Perm() != a.Mode.Perm() {
			changes = append(changes, FieldChange{"mode",
				b.Mode.Perm().String(), a.Mode.Perm().String()})
		}
		if b.Owner != a.Owner {
			changes = append(changes, FieldChange{"owner", b.Owner, a.Owner})
		}
		if b.Group != a.Group {
			changes = append(changes, FieldChange{"group", b.Group, a.Group})
		}
		if !b.ModTime.Equal(a.ModTime) {
			changes = append(changes, FieldChange{"mtime",
				b.ModTime.Format("2006-01-02 15:04:05"),
				a.ModTime.Format("2006-01-02 15:04:05")})
		}

		if len(changes) > 0 {
			res.Changed = append(res.Changed, FileDiff{Path: path, Changes: changes})
		} else {
			res.Unchanged = append(res.Unchanged, path)
		}
	}

	// Search new files
	for path := range actual {
		if _, ok := base[path]; !ok {
			res.OnlyInActual = append(res.OnlyInActual, path)
		}
	}

	return res
}
// Список файлов в каталоге
// func getDirectoryContents(dir string) []string {
    // var names []string
    // entries, err := os.ReadDir(dir);
    // if err != nil {
        // log.Println("Eror reading the directory: ", dir, err);
    // } else {
            // log.Println("Get the directory content:", dir)
    // }
        // //iterate through the directory entities and print out their name.
    // for _, entry := range(entries) {
        // log.Println("Finded file:", entry.Name())
        // names = append(names, entry.Name())
    // }
   // return names
// }

func FilesCheck(filesPath []string, previousFilesList FileMap) (FileMap, string) {

    // var previousFilesList FileMap

    currentFilesList := make(FileMap)
    
    for _, v := range filesPath {
        log.Println("Running CheckFiles for", v)
        // checklibs.FilesCheck(v)
        filesList, errs := GetFilesList(v)
        
        for path, meta := range filesList {
            currentFilesList[path] = meta
        }

        for _, e := range errs {
            fmt.Fprintln(os.Stderr, "warn:", e)
        }
    }

    if previousFilesList == nil {
        previousFilesList = maps.Clone(currentFilesList)
    }

    diff := CompareFilesList(previousFilesList, currentFilesList)
    out := PrintDiff(diff, false)
    // fmt.Println(out)

    // previousFilesList = maps.Clone(currentFilesList)
    return maps.Clone(currentFilesList), out
}


// FilesCheck traverses the tree and returns a map with metadata.
// Errors on individual elements do not interrupt the traversal but are collected in a separate slice.
func GetFilesList(dir string) (FileMap, []error) {
	result := make(FileMap)
	var errs []error

	// Кэш uid/gid → имя, чтобы не дёргать NSS на каждый файл.
	userCache := make(map[uint32]string)
	groupCache := make(map[uint32]string)

	resolveUser := func(uid uint32) string {
		if name, ok := userCache[uid]; ok {
			return name
		}
		name := fmt.Sprintf("uid=%d", uid)
		if u, err := user.LookupId(fmt.Sprint(uid)); err == nil {
			name = u.Username
		}
		userCache[uid] = name
		return name
	}

	resolveGroup := func(gid uint32) string {
		if name, ok := groupCache[gid]; ok {
			return name
		}
		name := fmt.Sprintf("gid=%d", gid)
		if g, err := user.LookupGroupId(fmt.Sprint(gid)); err == nil {
			name = g.Name
		}
		groupCache[gid] = name
		return name
	}

	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p, err))
			return nil // не прерываем обход
		}

		info, err := d.Info()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p, err))
			return nil
		}

		fileType := "other"
		switch {
		case info.IsDir():
			fileType = "dir"
		case info.Mode()&os.ModeSymlink != 0:
			fileType = "symlink"
		case info.Mode().IsRegular():
			fileType = "file"
		}

		meta := FileMeta{
			Path:    p,
			FileType:    fileType,
			Size:    info.Size(),
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
		}

		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			meta.Owner = resolveUser(stat.Uid)
			meta.Group = resolveGroup(stat.Gid)
		}

		result[p] = meta
		return nil
	})
	if err != nil {
		errs = append(errs, err)
	}

	return result, errs
}
