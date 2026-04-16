/// <reference path="../pb_data/types.d.ts" />
migrate((app) => {
  const collection = app.findCollectionByNameOrId("whatsapp_messages")

  collection.fields.add(new Field({
    "id": "msg_direction",
    "name": "direction",
    "type": "text",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": { "min": null, "max": null, "pattern": "" }
  }))

  collection.fields.add(new Field({
    "id": "msg_conversation_id",
    "name": "conversation_id",
    "type": "text",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": { "min": null, "max": null, "pattern": "" }
  }))

  app.save(collection)
}, (app) => {
  const collection = app.findCollectionByNameOrId("whatsapp_messages")

  const directionField = collection.fields.getByName("direction")
  collection.fields.remove(directionField.id)

  const convField = collection.fields.getByName("conversation_id")
  collection.fields.remove(convField.id)

  app.save(collection)
})
