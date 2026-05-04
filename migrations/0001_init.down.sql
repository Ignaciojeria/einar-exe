-- ============================================================
-- 0001_init — DOWN
-- ============================================================
-- Revierte la creación del schema de aplicación.
-- No dropeamos las extensiones: pueden ser usadas por otros objetos
-- (incluso futuros) y dropearlas en down genera más problemas que
-- soluciones. Si realmente quieres eliminarlas, hazlo a mano.
-- ============================================================

DROP TABLE IF EXISTS users;
