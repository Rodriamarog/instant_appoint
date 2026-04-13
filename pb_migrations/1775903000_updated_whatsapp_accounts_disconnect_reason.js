/// <reference path="../pb_data/types.d.ts" />
migrate((app) => {
  const collection = app.findCollectionByNameOrId("whatsapp_accounts")

  const field = new Field({
    "id": "disconnect_reason_field",
    "name": "disconnect_reason",
    "type": "text",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": { "min": null, "max": null, "pattern": "" }
  })

  collection.fields.add(field)
  app.save(collection)
}, (app) => {
  const collection = app.findCollectionByNameOrId("whatsapp_accounts")
  const field = collection.fields.getByName("disconnect_reason")
  collection.fields.remove(field.id)
  app.save(collection)
})
