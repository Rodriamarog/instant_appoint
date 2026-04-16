/// <reference path="../pb_data/types.d.ts" />
migrate((app) => {
  const collection = app.findCollectionByNameOrId("pbc_3975332653");

  const fields = [
    { name: "account_type", type: "text" },   // "web" | "business_api"
    { name: "waba_id", type: "text" },
    { name: "phone_number_id", type: "text" },
    { name: "access_token", type: "text" },
  ];

  for (const f of fields) {
    const field = new Field({
      "autogeneratePattern": "",
      "hidden": false,
      "max": 0,
      "min": 0,
      "name": f.name,
      "pattern": "",
      "presentable": false,
      "primaryKey": false,
      "required": false,
      "system": false,
      "type": f.type,
    });
    collection.fields.add(field);
  }

  return app.save(collection);
}, (app) => {
  const collection = app.findCollectionByNameOrId("pbc_3975332653");

  for (const name of ["account_type", "waba_id", "phone_number_id", "access_token"]) {
    collection.fields.removeByName(name);
  }

  return app.save(collection);
});
