package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// businessNameField is the one field this app adds to the stock users
// collection. Registration writes it and the reminder copy reads it, so a
// missing field turns every signup into a 400 "unknown field".
const businessNameField = "business_name"

func init() {
	m.Register(func(app core.App) error {
		// PocketBase's own 1640988000_init migration already creates the users
		// auth collection, and system migrations run first, so this is an
		// update in practice. The create branch exists so the collection is
		// still ours to define if upstream ever stops shipping it.
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			users = core.NewAuthCollection("users", "_pb_users_auth_")
			users.Fields.Add(
				&core.TextField{Name: "name", Max: 255},
				&core.FileField{
					Name:      "avatar",
					MaxSelect: 1,
					MimeTypes: []string{"image/jpeg", "image/png", "image/svg+xml", "image/gif", "image/webp"},
				},
				&core.AutodateField{Name: "created", OnCreate: true},
				&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
			)
			users.OAuth2.MappedFields.Name = "name"
			users.OAuth2.MappedFields.AvatarURL = "avatar"
		}

		if users.Fields.GetByName(businessNameField) == nil {
			users.Fields.Add(&core.TextField{Name: businessNameField, Max: 255})
		}

		// Stated here rather than inherited so this file is the only place the
		// rules live. A user may only ever see or touch their own row; anyone
		// may create one, which is what makes the public registration form
		// work without a superuser token.
		owner := "id = @request.auth.id"
		users.ListRule = types.Pointer(owner)
		users.ViewRule = types.Pointer(owner)
		users.CreateRule = types.Pointer("")
		users.UpdateRule = types.Pointer(owner)
		users.DeleteRule = types.Pointer(owner)

		return app.Save(users)
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return nil // already gone
		}

		users.Fields.RemoveByName(businessNameField)

		return app.Save(users)
	})
}
