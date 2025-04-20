package db

//go:generate go tool entgo.io/ent/cmd/ent generate --feature sql/versioned-migration --feature sql/modifier --feature sql/upsert --feature sql/lock ./schema --target ./ent --template ../lib/ent/tpl/tx_wrap.tmpl
