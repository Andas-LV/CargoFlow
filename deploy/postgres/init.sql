-- Runs once on the first start of the postgres container (empty volume).
-- One database per service: services never share a database.
CREATE DATABASE auth;
CREATE DATABASE shipment;
CREATE DATABASE tracking;
