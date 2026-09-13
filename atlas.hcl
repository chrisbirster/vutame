env "local" {
  url = "sqlite://vutame.db"
  src = "file://internal/dbschema/schema.sql"
  dev = "sqlite://file?mode=memory"
}

env "production" {
  url = getenv("VUTAME_ATLAS_URL")
  src = "file://internal/dbschema/schema.sql"
  dev = "sqlite://file?mode=memory"
}
