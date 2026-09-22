//************************************************* 
// A module for working with the system user list.
//************************************************* 

package mainlibs

import (
    "fmt"
    "os/user"
)

func	ResolveUser(uid uint32) string {
    userCache := make(map[uint32]string)
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

func ResolveGroup(gid uint32) string {
    groupCache := make(map[uint32]string)
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
