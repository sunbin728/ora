package ora_test

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	ora "gopkg.in/rana/ora.v5"
)

var testDb *sql.DB

func init() {
	var err error
	if testDb, err = sql.Open("ora", os.Getenv("GO_ORA_DRV_TEST_USERNAME")+"/"+os.Getenv("GO_ORA_DRV_TEST_PASSWORD")+"@"+os.Getenv("GO_ORA_DRV_TEST_DB")); err != nil {
		fmt.Println("ERROR")
		panic(err)
	}
}

func TestSelect(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	const num = 1000
	rows, err := testDb.QueryContext(ctx, "SELECT object_name, object_type, object_id, created FROM all_objects WHERE ROWNUM < NVL(:alpha, 2) ORDER BY object_id", sql.Named("alpha", num))
	//rows, err := testDb.QueryContext(ctx, "SELECT object_name, object_type, object_id, created FROM all_objects WHERE ROWNUM < 1000 ORDER BY object_id")
	if err != nil {
		t.Fatalf("%+v", err)
	}
	n, oldOid := 0, int64(0)
	for rows.Next() {
		var tbl, typ string
		var oid int64
		var created time.Time
		if err := rows.Scan(&tbl, &typ, &oid, &created); err != nil {
			t.Fatal(err)
		}
		t.Log(tbl, typ, oid, created)
		if tbl == "" {
			t.Fatal("empty tbl")
		}
		n++
		if oldOid > oid {
			t.Errorf("got oid=%d, wanted sth < %d.", oid, oldOid)
		}
		oldOid = oid
	}
	if n != num-1 {
		t.Errorf("got %d rows, wanted %d", n, num-1)
	}
}
func TestExecuteMany(t *testing.T) {
	t.Parallel()
	testDb.Exec("CREATE TABLE test_em (f_int INTEGER, f_num NUMBER, f_num_6 NUMBER(6), F_num_5_2 NUMBER(5,2), f_vc VARCHAR2(30), F_dt DATE)")
	defer testDb.Exec("DROP TABLE test_em")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	const num = 1000
	ints := make([]int, num)
	nums := make([]string, num)
	int32s := make([]int32, num)
	floats := make([]float32, num)
	strs := make([]string, num)
	dates := make([]time.Time, num)
	now := time.Now()
	for i := range nums {
		ints[i] = i << 1
		nums[i] = fmt.Sprintf("%d", i)
		int32s[i] = int32(i)
		floats[i] = float32(i) / float32(3.14)
		strs[i] = fmt.Sprintf("%x", i)
		dates[i] = now.Add(-time.Duration(i) * time.Hour)
	}
	for i, tc := range []struct {
		Name  string
		Value interface{}
	}{
		{"f_int", ints},
		{"f_num", nums},
		{"f_num_6", int32s},
		{"f_num_5_2", floats},
		{"f_vc", strs},
		{"f_dt", dates},
	} {
		res, err := testDb.ExecContext(ctx,
			"INSERT INTO test_em ("+tc.Name+") VALUES (:1)",
			tc.Value)
		if err != nil {
			t.Fatalf("%d. %+v: %#v", i, tc, err)
		}
		ra, err := res.RowsAffected()
		if err != nil {
			t.Error(err)
		} else if ra != num {
			t.Errorf("%d. %q: wanted %d rows, got %d", i, tc.Name, num, ra)
		}
	}

	res, err := testDb.ExecContext(ctx,
		`INSERT INTO test_em
		  (f_int, f_num, f_num_6, F_num_5_2, F_vc, F_dt)
		  VALUES
		  (:1, :2, :3, :4, :5, :6)`,
		ints, nums, nums, nums, strs, dates)
	if err != nil {
		t.Fatalf("%#v", err)
	}
	ra, err := res.RowsAffected()
	if err != nil {
		t.Error(err)
	} else if ra != num {
		t.Errorf("wanted %d rows, got %d", num, ra)
	}
}
func TestReadWriteLob(t *testing.T) {
	t.Parallel()
	testDb.Exec("CREATE TABLE test_lob (f_id NUMBER(6), f_blob BLOB, f_clob CLOB)")
	defer testDb.Exec("DROP TABLE test_lob")

	stmt, err := testDb.Prepare("INSERT INTO test_lob (F_id, f_blob, F_clob) VALUES (:1, :2, :3)")
	if err != nil {
		t.Fatal(err)
	}
	defer stmt.Close()

	for tN, tC := range []struct {
		Bytes  []byte
		String string
	}{
		{[]byte{0, 1, 2, 3, 4, 5}, "12345"},
	} {

		if _, err := stmt.Exec(tN*2, tC.Bytes, tC.String); err != nil {
			t.Errorf("%d/1. (%v, %q): %v", tN, tC.Bytes, tC.String, err)
			continue
		}
		if _, err := stmt.Exec(tN*2+1,
			ora.Lob{Reader: bytes.NewReader(tC.Bytes)},
			ora.Lob{Reader: strings.NewReader(tC.String), IsClob: true},
		); err != nil {
			t.Errorf("%d/2. (%v, %q): %v", tN, tC.Bytes, tC.String, err)
		}
	}
}