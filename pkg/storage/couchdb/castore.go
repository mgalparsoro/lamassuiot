package couchdb

import (
	"context"
	"net/url"

	_ "github.com/go-kivik/couchdb/v4" // The CouchDB driver
	kivik "github.com/go-kivik/kivik/v4"
	"github.com/lamassuiot/lamassuiot/pkg/helppers"
	"github.com/lamassuiot/lamassuiot/pkg/models"
	"github.com/lamassuiot/lamassuiot/pkg/resources"
	"github.com/lamassuiot/lamassuiot/pkg/storage"
)

const caDBName = "certificate-authroty"

type CouchDBCAStorage struct {
	client  *kivik.Client
	querier *couchDBQuerier[models.CACertificate]
}

func NewCouchCARepository(couchURL url.URL, username, password string) (storage.CACertificatesRepo, error) {
	client, err := createCouchDBConnection(couchURL, username, password, []string{caDBName})
	if err != nil {
		return nil, err
	}

	querier := newCouchDBQuerier[models.CACertificate](client.DB(caDBName))
	querier.CreateBasicCounterView()

	return &CouchDBCAStorage{
		client:  client,
		querier: &querier,
	}, nil
}

func (db *CouchDBCAStorage) Count(ctx context.Context) (int, error) {
	return db.querier.Count()
}

func (db *CouchDBCAStorage) SelectByType(ctx context.Context, CAType models.CAType, exhaustiveRun bool, applyFunc func(*models.CACertificate), queryParams *resources.QueryParameters, extraOpts map[string]interface{}) (string, error) {
	opts := map[string]interface{}{
		"type": CAType,
	}
	return db.querier.SelecAll(queryParams, helppers.MergeMaps(&extraOpts, &opts), exhaustiveRun, applyFunc)
}

func (db *CouchDBCAStorage) SelectAll(ctx context.Context, exhaustiveRun bool, applyFunc func(*models.CACertificate), queryParams *resources.QueryParameters, extraOpts map[string]interface{}) (string, error) {
	return db.querier.SelecAll(queryParams, &extraOpts, exhaustiveRun, applyFunc)
}

func (db *CouchDBCAStorage) Select(ctx context.Context, id string) (*models.CACertificate, error) {
	return db.querier.SelectByID(id)
}

func (db *CouchDBCAStorage) Insert(ctx context.Context, caCertificate *models.CACertificate) (*models.CACertificate, error) {
	return db.querier.Insert(*caCertificate, caCertificate.ID)
}

func (db *CouchDBCAStorage) Update(ctx context.Context, caCertificate *models.CACertificate) (*models.CACertificate, error) {
	return db.querier.Update(*caCertificate, caCertificate.ID)
}