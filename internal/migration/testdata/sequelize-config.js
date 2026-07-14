module.exports = {
  production: {
    dialect: "postgres",
    database: "alice_prod",
    username: "alice_prod_user",
    password: "synthetic-production-secret",
    host: "postgres-prod",
    pool: {
      max: 20,
      min: 0,
      acquire: 30000,
      idle: 10000,
    },
    logging: false,
  },
  test: null,
  development: {
    dialect: "postgres",
    database: "alice_development",
    username: "alice_development_user",
    password: "synthetic-development-secret",
    host: process.env.DEV_DB_HOST || "127.0.0.1",
    port: process.env.DEV_DB_PORT ?? 5435,
    pool: {
      max: 5,
      min: 0,
      flags: [true, false, null],
    },
    logging: true,
  },
};
