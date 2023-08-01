package couchdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	_ "github.com/go-kivik/couchdb" // The CouchDB driver
	"github.com/go-kivik/couchdb/v4"
	"github.com/go-kivik/couchdb/v4/chttp"
	"github.com/go-kivik/kivik/v4"
	"github.com/lamassuiot/lamassuiot/pkg/v3/config"
	"github.com/lamassuiot/lamassuiot/pkg/v3/helpers"
	"github.com/lamassuiot/lamassuiot/pkg/v3/resources"
	log "github.com/sirupsen/logrus"
)

func CreateCouchDBConnection(cfg config.HTTPConnection, username, password string) (*kivik.Client, error) {
	address := fmt.Sprintf("%s://%s:%s@%s:%d%s", cfg.Protocol, username, password, cfg.Hostname, cfg.Port, cfg.BasePath)

	httpCli, err := helpers.BuildHTTPClientWithTLSOptions(&http.Client{}, cfg.TLSConfig)
	httpCli, err = helpers.BuildHTTPClientWithloggger(httpCli, "CouchDB")
	if err != nil {
		return nil, err
	}

	kivikOpts := kivik.Options{
		couchdb.OptionHTTPClient: httpCli,
	}

	client, err := kivik.New("couch", address, kivikOpts)
	if err != nil {
		return nil, err
	}

	ping, err := client.Ping(context.Background())
	if err != nil {
		return nil, err
	}

	if !ping {
		return nil, fmt.Errorf("no connectivity with couchdb")
	}

	return client, nil
}

func CheckAndCreateDB(client *kivik.Client, db string) error {
	if exists, err := client.DBExists(context.TODO(), db); err == nil && !exists {
		log.Infof("db does not exist. Creating db: %s", db)
		if err := client.CreateDB(context.TODO(), db); err != nil {
			log.Error(fmt.Sprintf("could not create db %s: %s", db, err))
			return err
		}
	}

	return nil
}

type couchDBQuerier[E any] struct {
	*kivik.DB
}

func newCouchDBQuerier[E any](db *kivik.DB) couchDBQuerier[E] {
	return couchDBQuerier[E]{
		DB: db,
	}
}

func (db *couchDBQuerier[E]) CreateBasicCounterView() error {
	querier := newCouchDBQuerier[map[string]interface{}](db.DB)
	_, err := querier.DB.Put(context.TODO(), "_design/utils", map[string]interface{}{
		"_id": "_design/utils",
		"views": map[string]interface{}{
			"count": map[string]interface{}{
				"map":    "function(doc) { emit(doc._id) }",
				"reduce": "_count",
			},
		},
	})

	if err != nil {
		return fmt.Errorf("error while creating design doc: %s", err)
	}

	return nil
}

func (db *couchDBQuerier[E]) Count() (int, error) {
	opts := map[string]interface{}{}
	rows := db.Query(context.TODO(), "_design/utils", "_view/count", opts)
	if err := rows.Err(); err != nil {
		return -1, err
	}

	rows.Next()
	var result int
	err := rows.ScanValue(&result)
	if err != nil {
		return -1, err
	}

	return result, nil

}

func (db *couchDBQuerier[E]) SelectAll(queryParams *resources.QueryParameters, extraOpts *map[string]interface{}, exhaustiveRun bool, applyFunc func(elem *E)) (string, error) {
	nextBookmark := ""
	opts := map[string]interface{}{
		"selector": map[string]interface{}{
			"_id": map[string]string{
				"$ne": "",
			},
		},
	}

	if queryParams != nil {
		if queryParams.Sort.SortField != "" {
			opts["sort"] = []map[string]string{
				{queryParams.Sort.SortField: string(queryParams.Sort.SortMode)},
			}
		}

		if queryParams.Pagination.NextBookmark != "" {
			nextBookmark = queryParams.Pagination.NextBookmark
		}
	}

	iterCounter := 0

	for key, val := range *extraOpts {
		opts[key] = val
	}

	continueIter := true
	for continueIter {

		bookmark, elems, err := getElements[*E](db.DB, nextBookmark, opts)
		if err != nil {
			return "", err
		}

		for _, elem := range elems {
			applyFunc(elem)
		}

		if !exhaustiveRun {
			continueIter = false
		}

		if bookmark == "nil" {
			nextBookmark = ""
			continueIter = false
		} else if bookmark != nextBookmark {
			nextBookmark = bookmark
		} else {
			nextBookmark = ""
			continueIter = false
		}

		// fmt.Printf("  >>  running iter [%d]. Number of items in resp [%d]. Has Next [%t]. NextBookmark [%s]\n", iterCounter, len(elems), continueIter, nextBookmark)
		iterCounter++
	}

	return nextBookmark, nil
}

func (db *couchDBQuerier[E]) SelectByID(elemID string) (*E, error) {
	rs := db.Get(context.Background(), elemID)
	if rs.Err() != nil {
		return nil, rs.Err()
	}

	rs.Next()
	var elem E
	if err := rs.ScanDoc(&elem); err != nil {
		return nil, err
	}

	return &elem, nil
}

func (db *couchDBQuerier[E]) Exists(elemID string) (bool, error) {
	rs := db.Get(context.Background(), elemID)
	err := rs.Err()
	if err != nil {
		switch t := err.(type) {
		case *chttp.HTTPError:
			resp := err.(*chttp.HTTPError)
			if resp.Response.StatusCode == http.StatusNotFound {
				return false, nil
			} else {
				return false, err
			}
		default:
			fmt.Println(t)
			return false, err
		}
	}

	return rs.Next(), nil
}

func (db *couchDBQuerier[E]) Insert(elem E, elemID string) (*E, error) {
	_, err := db.Put(context.Background(), elemID, elem)
	if err != nil {
		return nil, err
	}

	return db.SelectByID(elemID)
}

func (db *couchDBQuerier[E]) Update(elem E, elemID string) (*E, error) {
	rs := db.Get(context.Background(), elemID)
	if rs.Err() != nil {
		return nil, rs.Err()
	}

	rs.Next()
	var prevElem map[string]interface{}
	if err := rs.ScanDoc(&prevElem); err != nil {
		return nil, err
	}

	marshalElem, err := json.Marshal(elem)
	if err != nil {
		return nil, err
	}

	var newElem map[string]interface{}
	err = json.Unmarshal(marshalElem, &newElem)
	if err != nil {
		return nil, err
	}

	newElem["_rev"] = prevElem["_rev"]
	_, err = db.Put(context.Background(), elemID, newElem)
	if err != nil {
		return nil, err
	}

	return db.SelectByID(elemID)
}

func getElements[E any](db *kivik.DB, bookmark string, opts map[string]interface{}) (string, []E, error) {
	ctx := context.Background()

	if bookmark != "" {
		opts["bookmark"] = bookmark
	}

	rs := db.Find(ctx, opts)
	if rs.Err() != nil {
		return "", []E{}, rs.Err()
	}

	elements := []E{}

	for rs.Next() {
		var element E
		if err := rs.ScanDoc(&element); err != nil {
			log.Warnf("error while processing element in result set:", err)
			continue
		}
		elements = append(elements, element)
	}

	finisthResult, err := rs.Finish()
	if err != nil {
		return "", []E{}, rs.Err()
	}

	return finisthResult.Bookmark, elements, nil
}